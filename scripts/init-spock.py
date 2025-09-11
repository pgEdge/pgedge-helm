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
    """Return list of CNPG cluster names in the namespace."""
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

def wait_for_clusters(namespace, nodes):
    """Wait until all CNPG clusters are Ready."""
    config.load_incluster_config()
    api = client.CustomObjectsApi()
    while True:
        ready = True
        for node in nodes:
            name = "pgedge-" + node["name"]
            try:
                cr = api.get_namespaced_custom_object(
                    group="postgresql.cnpg.io",
                    version="v1",
                    namespace=namespace,
                    plural="clusters",
                    name=name,
                )
                phase = cr.get("status", {}).get("phase")
                if phase != "Cluster in healthy state":
                    print(f"⏳ Cluster {name} not ready (phase={phase})")
                    ready = False
            except Exception as e:
                print(f"❌ Error fetching {name}: {e}")
                ready = False
        if ready:
            print("✅ All CNPG clusters in namespace ready")
            return
        time.sleep(5)

def wait_ready(host, db_name, admin_user, admin_password):
    """Wait until Postgres accepts connections."""

    while True:
        try:
            conn = psycopg2.connect(
                dbname=db_name,
                user=admin_user,
                password=admin_password,
                host=host,
                connect_timeout=3,
            )
            conn.close()
            print(f"✅ {host} is accepting connections")
            return
        except Exception as e:
            print(f"⏳ waiting for {host}: {e}")
            time.sleep(3)

def run_sql(host, db_name, admin_user, admin_password, statement, autocommit=False, ignore_duplicate=True):
    """Run a SQL statement on the given host."""

    with psycopg2.connect(
        dbname=db_name,
        user=admin_user,
        password=admin_password,
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
    admin_user = os.environ["ADMIN_USER"]
    admin_password = os.environ["ADMIN_PASSWORD"]
    pgedge_user = os.environ["PGEDGE_USER"]
    pgedge_password = os.environ["PGEDGE_PASSWORD"]
    namespace = os.environ.get("NAMESPACE", "default")

    with open("/config/pgedge.json") as f:
        nodes = json.load(f)

    # You are guaranteed only "name" and "hostname"
    for node in nodes:
        print(node["name"], node["hostname"])

    # Step 1: Wait for any nodes on this cluster to become ready
    clusters = get_clusters(namespace)
    print(f"Found clusters in namespace {namespace}: {clusters}")
    wait_for_clusters(namespace, nodes)

    # Step 2: Wait for all clusters to accept connections across all nodes
    for node in nodes:
        wait_ready(node["hostname"], db_name, admin_user, admin_password)

    # Step 3: Create pgedge user
    for node in nodes:
        stmt = f"""
            SELECT spock.repair_mode('True');
            CREATE ROLE {pgedge_user} WITH LOGIN SUPERUSER REPLICATION PASSWORD '{pgedge_password}';
        """
        run_sql(node["hostname"], db_name, admin_user, admin_password, stmt)
        print(f"👤 Created user {pgedge_user} on {node['name']}")

    # Step 4: Create spock nodes
    for node in nodes:
        stmt = f"""
            SELECT spock.repair_mode('True');
            SELECT spock.node_create(
                node_name := '{node["name"]}',
                dsn := 'host={node["hostname"]} dbname={db_name} user={pgedge_user} password={pgedge_password} port=5432'
            )
            WHERE '{node["name"]}' NOT IN (SELECT node_name FROM spock.node);
        """
        run_sql(node["hostname"], db_name, admin_user, admin_password, stmt)
        print(f"🖥️ Created spock node {node['name']} on {node['hostname']}")

    forward_origins = "{}"
    replication_sets = "{default, default_insert_only, ddl_sql}"

    # Step 5: Wire subscriptions between every pair of spock nodes
    for src in nodes:
        for dst in nodes:
            if src["name"] != dst["name"]:
                other_dsn = f"host={dst['hostname']} dbname={db_name} user={pgedge_user} password={pgedge_password} port=5432"
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
                        enabled := 'false'
                    )
                    WHERE '{sub_name}' NOT IN (SELECT sub_name FROM spock.subscription);
                """
                run_sql(src["hostname"], db_name, admin_user, admin_password, stmt)
                print(f"🔗 Created spock subscription {sub_name} on {src['name']}")

    # Step 6: Create replication slots for spock subscriptions to absorb
    for src in nodes:
        for dst in nodes:
            if src["name"] != dst["name"]:
                # spk_app_n1_sub_n2_n1
                slot_name = f"spk_{db_name}_{dst['name']}_sub_{src['name']}_{dst['name']}".replace("-", "_")
                stmt = f"SELECT pg_create_logical_replication_slot('{slot_name}', 'spock_output', false, false, true)"
                run_sql(dst["hostname"], db_name, admin_user, admin_password, stmt)
                print(f"🪑 Created replication slot {slot_name} on {dst['name']}")

    # Step 7: Enable all subscriptions
    for src in nodes:
        for dst in nodes:
            if src["name"] != dst["name"]:
                sub_name = f"sub_{src['name']}_{dst['name']}".replace("-", "_")
                stmt = f"SELECT spock.sub_enable(subscription_name := '{sub_name}')"
                run_sql(src["hostname"], db_name, admin_user, admin_password, stmt, autocommit=True)
                print(f"✅ Enabled spock subscription {sub_name} on {src['name']}")



    print("🎉 Spock nodes and subscriptions successfully initialized")


if __name__ == "__main__":
    main()