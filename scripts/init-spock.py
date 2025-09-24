#!/usr/bin/env python3
import json
import os
import sys
import time
import psycopg2
from psycopg2 import errors
from kubernetes import client, config
import json


def get_clusters(namespace):
    """Return list of CloudNativePG cluster names in the namespace."""
    config.load_incluster_config()
    api = client.CustomObjectsApi()
    objs = api.list_namespaced_custom_object(
        group="postgresql.cnpg.io",
        version="v1",
        namespace=namespace,
        plural="clusters",
    )
    clusters = [item["metadata"]["name"] for item in objs.get("items", [])]

    print(f"🔎 Found clusters in namespace {namespace}: {clusters}")

    return sorted(clusters)


def wait_for_clusters(namespace, clusters):
    """Wait until all CloudNativePG clusters are Ready."""
    config.load_incluster_config()
    api = client.CustomObjectsApi()
    while True:
        ready = True
        for cluster in clusters:
            try:
                cr = api.get_namespaced_custom_object(
                    group="postgresql.cnpg.io",
                    version="v1",
                    namespace=namespace,
                    plural="clusters",
                    name=cluster,
                )
                phase = cr.get("status", {}).get("phase")
                if phase != "Cluster in healthy state":
                    print(f"⏳ Cluster {cluster} not ready (phase={phase})")
                    ready = False
            except Exception as e:
                print(f"❌ Error fetching {cluster}: {e}")
                ready = False
        if ready:
            print("✅ All CloudNativePG clusters in namespace ready")
            return
        time.sleep(5)


def wait_ready(node, db_name, user):
    """Wait until Postgres accepts connections."""

    while True:
        try:
            conn = get_conn(node["hostname"], db_name, user)
            conn.close()
            print(f"✅ {node['hostname']} is accepting connections")
            return
        except Exception as e:
            print(f"⏳ waiting for {node['hostname']}: {e}")
            time.sleep(3)


def get_conn(host, db_name, user):
    """Get a psycopg2 connection to the given host."""
    return psycopg2.connect(
        dbname=db_name,
        user=user,
        sslmode="require",
        sslcert="/certificates/admin/tls.crt",
        sslkey="/certificates/admin/tls.key",
        host=host,
        connect_timeout=3,
    )


def run_sql(
    host, db_name, admin_user, statement, autocommit=False, ignore_duplicate=True
):
    """Run a SQL statement on the given host. Returns number of rows modified."""

    with get_conn(host, db_name, admin_user) as conn:
        conn.autocommit = autocommit
        with conn.cursor() as cur:
            try:
                cur.execute(statement)
                rows_modified = cur.rowcount
                if not autocommit:
                    conn.commit()
                return rows_modified
            except errors.DuplicateObject:
                if ignore_duplicate:
                    print(f"\tℹ️ Already exists on {host}")
                else:
                    raise
            except Exception as e:
                if not autocommit:
                    conn.rollback()
                print(f"❌ Error on {host}: {e}")
                raise


def create_subscription(db_name, admin_user, pgedge_user, src_node, dst_node):
    forward_origins = "{}"
    replication_sets = "{default, default_insert_only, ddl_sql}"

    ssl_settings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"
    other_dsn = f"host={dst_node['hostname']} dbname={db_name} user={pgedge_user} {ssl_settings} port=5432"
    sub_name = f"sub_{src_node['name']}_{dst_node['name']}".replace("-", "_")
    stmt = f"""
                    SELECT spock.sub_create(
                        subscription_name := '{sub_name}',
                        provider_dsn := '{other_dsn}',
                        replication_sets := '{replication_sets}',
                        forward_origins := '{forward_origins}',
                        synchronize_structure := 'false',
                        synchronize_data := 'false',
                        apply_delay := '0',
                        enabled := 'true'
                    )
                    WHERE '{sub_name}' NOT IN (SELECT sub_name FROM spock.subscription);
                """
    row_count = run_sql(src_node["hostname"], db_name, admin_user, stmt)
    if row_count and row_count > 0:
        print(f"🔗 Created spock subscription {sub_name} on {src_node['name']}")


def create_node(node, db_name, admin_user, pgedge_user):
    ssl_settings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"
    stmt = f"""
            SELECT spock.repair_mode('True');
            SELECT spock.node_create(
                node_name := '{node["name"]}',
                dsn := 'host={node["hostname"]} dbname={db_name} user={pgedge_user} {ssl_settings} port=5432'
            )
            WHERE '{node["name"]}' NOT IN (SELECT node_name FROM spock.node);
        """
    row_count = run_sql(node["hostname"], db_name, admin_user, stmt)
    if row_count and row_count > 0:
        print(f"🖖 Created spock node {node['name']} on {node['hostname']}")


def drop_recovered_node(node, db_name, admin_user):
    # If a recovery occurs, the nodes and subscriptions will come over from the source
    # Cluster, and need to be removed before recreating the correct values.
    # This will not affect nodes / subscriptions which should exist on this node.

    stmt = f"""
            SELECT spock.repair_mode('True');
            SELECT spock.sub_drop(sub_name, true)
              FROM spock.subscription
             WHERE sub_target IN (
            SELECT l.node_id
              FROM spock.local_node l
              JOIN spock.node n ON n.node_id = l.node_id
             WHERE n.node_name != '{node["name"]}'
             );
            SELECT spock.node_drop(node_name, true)
              FROM spock.local_node l
              JOIN spock.node n ON n.node_id = l.node_id
             WHERE n.node_name != '{node["name"]}';
        """
    row_count = run_sql(node["hostname"], db_name, admin_user, stmt)

    if row_count and row_count > 0:
        print(f"🗑️ Dropped existing spock node on {node['hostname']}")
        return True

    return False


def create_pgedge_user(node, db_name, admin_user, pgedge_user):
    print(f"👤 Creating user {pgedge_user} on {node['name']}")
    stmt = f"""
            SELECT spock.repair_mode('True');
            CREATE ROLE {pgedge_user} WITH LOGIN SUPERUSER REPLICATION;
        """
    run_sql(node["hostname"], db_name, admin_user, stmt, ignore_duplicate=True)


def drop_removed_nodes(node, db_name, admin_user, node_names):
    stmt = f"""
            SELECT spock.repair_mode('True');
            
            SELECT spock.sub_drop(sub_name, true)
              FROM spock.subscription
             WHERE sub_origin IN (
            SELECT n.node_id
              FROM spock.node n
             WHERE n.node_name NOT IN ({', '.join(f"'{name}'" for name in node_names)})
             );
            SELECT spock.node_drop(node_name, true)
              FROM spock.node n
             WHERE n.node_name NOT IN ({', '.join(f"'{name}'" for name in node_names)});
        """
    row_count = run_sql(node["hostname"], db_name, admin_user, stmt)
    if row_count and row_count > 0:
        print(f"🗑️ Dropped removed spock nodes on {node['hostname']}")

def backup_spock_repsets(node, db_name, admin_user):
    with get_conn(node["hostname"], db_name, admin_user) as conn:
        with conn.cursor() as cur:
            cur.execute("SET log_statement = 'none';")
            try:
                
                cleanup_stmt = f"""
                    SELECT spock.repair_mode('True');

                    DROP TABLE IF EXISTS spock.replication_set_table_backup;
                    DROP TABLE IF EXISTS spock.replication_set_backup;
                """
                cur.execute(cleanup_stmt)

                backup_stmt = f"""
                    SELECT spock.repair_mode('True');

                    CREATE TABLE spock.replication_set_backup AS 
                       SELECT * FROM spock.replication_set;
                    CREATE TABLE spock.replication_set_table_backup AS 
                       SELECT * FROM spock.replication_set_table;
                """
                cur.execute(backup_stmt)
                
                conn.commit()
                print(f"Successfully backed up spock replication sets for node {node['name']}")
            except Exception as e:
                conn.rollback()
                print(f"Warning: Failed to backup spock replication sets for node {node['name']}: {str(e)}")
                raise

def restore_spock_repsets(node, db_name, admin_user):
    with get_conn(node["hostname"], db_name, admin_user) as conn:
        with conn.cursor() as cur:
            cur.execute("SET log_statement = 'none';")
            
            cur.execute("""
                SELECT EXISTS (
                    SELECT 1 
                    FROM information_schema.tables 
                    WHERE table_schema = 'spock' 
                    AND table_name = 'replication_set_backup'
                );
            """)
            if not cur.fetchone()[0]:
                print(f"No replication set backup found to restore for node {node['name']}")
                return

            try:
                cur.execute("""
                    SELECT spock.repset_create(
                        rs.set_name, 
                        rs.replicate_insert, 
                        rs.replicate_update, 
                        rs.replicate_delete, 
                        rs.replicate_truncate
                    ) 
                    FROM spock.replication_set_backup rs 
                    WHERE rs.set_name NOT IN ('default', 'default_insert_only', 'ddl_sql');
                """)
                
                cur.execute("""
                    SELECT 
                        rs.set_name,
                        rst.set_reloid::regclass::text as table_name,
                        rst.set_att_list,
                        rst.set_row_filter
                    FROM spock.replication_set_table_backup rst 
                    LEFT JOIN spock.replication_set_backup rs ON rst.set_id = rs.set_id
                    -- Check that the OID exists and is a valid table/view
                    WHERE EXISTS (
                        SELECT 1 
                        FROM pg_class c 
                        JOIN pg_namespace n ON n.oid = c.relnamespace
                        WHERE c.oid = rst.set_reloid 
                        AND c.relkind IN ('r', 'v')  -- Only tables and views
                    );
                """)
                
                for row in cur.fetchall():
                    try:
                        set_name, table_name, att_list, row_filter = row
                        cur.execute(
                            "SELECT spock.repset_add_table(%s, %s, false, %s, %s);",
                            (set_name, table_name, att_list, row_filter)
                        )
                        print(f"Added table {table_name} to replication set {set_name} on node {node['name']}")
                    except Exception as e:
                        print(f"Warning: Failed to add table {table_name} to replication set {set_name} on node {node['name']}: {str(e)}")
                        continue

                conn.commit()
                print(f"Successfully restored spock replication sets for node {node['name']}")
            except Exception as e:
                conn.rollback()
                print(f"Error during replication set restore for node {node['name']}: {str(e)}")
                raise
            finally:
                try:
                    conn.commit()
                    cur.execute("SET log_statement = 'none';")
                    cleanup_stmt = f"""
                        SELECT spock.repair_mode('True');

                        DROP TABLE IF EXISTS spock.replication_set_table_backup;
                        DROP TABLE IF EXISTS spock.replication_set_backup;
                    """
                    cur.execute(cleanup_stmt)
                    conn.commit()
                except Exception as e:
                    conn.rollback()
                    print(f"Warning: Failed to clean up backup tables for node {node['name']}: {str(e)}")

def main():
    db_name = os.environ["DB_NAME"]
    admin_user = "admin"
    pgedge_user = "pgedge"
    namespace = os.environ.get("NAMESPACE", "default")

    with open("/config/pgedge.json") as f:
        nodes = json.load(f)

    print(f"🎯 Configuring Spock for nodes:")
    for node in nodes:
        print(f" - 🖖 Node: {node['name']} | Hostname: {node['hostname']}")

    # Step 1: Wait for any nodes to become ready
    clusters = get_clusters(namespace)
    wait_for_clusters(namespace, clusters)

    # Step 2: Wait for all nodes to accept connections
    for node in nodes:
        wait_ready(node, db_name, admin_user)

    # Step 3: Create pgedge user (if not exists) on all nodes
    for node in nodes:
        create_pgedge_user(node, db_name, admin_user, pgedge_user)

    # Step 4: Drop spock nodes and subscriptions which no longer exist
    current_node_names = [node["name"] for node in nodes]
    for node in nodes:
        drop_removed_nodes(node, db_name, admin_user, current_node_names)

    # Step 5: Recreate the spock nodes, dropping any recovered nodes first
    # Restore replication sets after recreating nodes if they were dropped
    for node in nodes:
        create_node(node, db_name, admin_user, pgedge_user)
        
    # Step 6: Wire subscriptions between every pair of spock nodes
    for src_node in nodes:
        for dst_node in nodes:
            if src_node["name"] != dst_node["name"]:
                create_subscription(
                    db_name, admin_user, pgedge_user, src_node, dst_node
                )

    print("🎉 Spock configuration successfully applied")


if __name__ == "__main__":
    main()
