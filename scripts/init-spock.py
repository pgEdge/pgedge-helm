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


def wait_ready(host, db_name, user):
    """Wait until Postgres accepts connections."""

    while True:
        try:
            conn = psycopg2.connect(
                dbname=db_name,
                host=host,
                connect_timeout=3,
                user=user,
                sslmode="require",
                sslcert="/certificates/admin/tls.crt",
                sslkey="/certificates/admin/tls.key",
            )
            conn.close()
            print(f"✅ {host} is accepting connections")
            return
        except Exception as e:
            print(f"⏳ waiting for {host}: {e}")
            time.sleep(3)


def run_sql(
    host, db_name, admin_user, statement, autocommit=False, ignore_duplicate=True
):
    """Run a SQL statement on the given host."""

    with psycopg2.connect(
        dbname=db_name,
        user=admin_user,
        sslmode="require",
        sslcert="/certificates/admin/tls.crt",
        sslkey="/certificates/admin/tls.key",
        host=host,
        connect_timeout=3,
    ) as conn:
        conn.autocommit = autocommit
        with conn.cursor() as cur:
            try:
                cur.execute(statement)
                if not autocommit:
                    conn.commit()
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


def main():
    db_name = os.environ["DB_NAME"]
    admin_user = "admin"
    pgedge_user = "pgedge"
    namespace = os.environ.get("NAMESPACE", "default")

    with open("/config/pgedge.json") as f:
        nodes = json.load(f)

    # You are guaranteed only "name" and "hostname"
    for node in nodes:
        print(node["name"], node["hostname"])

    # Step 1: Wait for any nodes on this cluster to become ready
    clusters = get_clusters(namespace)
    print(f"Found clusters in namespace {namespace}: {clusters}")
    wait_for_clusters(namespace, clusters)

    # Step 2: Wait for all clusters to accept connections across all nodes
    for node in nodes:
        wait_ready(node["hostname"], db_name, admin_user)

    # Step 3: Create pgedge user
    for node in nodes:
        stmt = f"""
            SELECT spock.repair_mode('True');
            CREATE ROLE {pgedge_user} WITH LOGIN SUPERUSER REPLICATION;
        """
        run_sql(node["hostname"], db_name, admin_user, stmt)
        print(f"👤 Created user {pgedge_user} on {node['name']}")

    # Step 4: Drop spock nodes and subscriptions which no longer exist
    node_names = [node["name"] for node in nodes]
    for node in nodes:
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
        run_sql(node["hostname"], db_name, admin_user, stmt)

    # Step 5: Drop spock nodes and subscriptions which may have come over in a restore
    # If a restore occurs, the nodes and subscriptions will come over from the source
    # Cluster, and need to be removed before recreating the correct values. 
    # This will not affect nodes / subscriptions which should exist on this node.
    for node in nodes:
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
        run_sql(node["hostname"], db_name, admin_user, stmt)

    # Step 6: Create spock nodes
    for node in nodes:
        ssl_settings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"
        stmt = f"""
            SELECT spock.repair_mode('True');

            SELECT spock.node_create(
                node_name := '{node["name"]}',
                dsn := 'host={node["hostname"]} dbname={db_name} user={pgedge_user} {ssl_settings} port=5432'
            )
            WHERE '{node["name"]}' NOT IN (SELECT node_name FROM spock.node);
        """
        run_sql(node["hostname"], db_name, admin_user, stmt)
        print(f"🖖 Created spock node {node['name']} on {node['hostname']}")

    forward_origins = "{}"
    replication_sets = "{default, default_insert_only, ddl_sql}"

    # Step 7: Wire subscriptions between every pair of spock nodes
    for src in nodes:
        for dst in nodes:
            if src["name"] != dst["name"]:
                ssl_settings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"
                other_dsn = f"host={dst['hostname']} dbname={db_name} user={pgedge_user} {ssl_settings} port=5432"
                sub_name = f"sub_{src['name']}_{dst['name']}".replace("-", "_")
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
                run_sql(src["hostname"], db_name, admin_user, stmt)
                print(f"🔗 Created spock subscription {sub_name} on {src['name']}")

    print("🎉 Spock nodes and subscriptions successfully initialized")


if __name__ == "__main__":
    main()
