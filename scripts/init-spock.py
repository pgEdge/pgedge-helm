#!/usr/bin/env python3
import os
import sys
import time
import psycopg2
from psycopg2 import errors
from kubernetes import client, config

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

def wait_for_clusters(namespace, clusters):
    """Wait until all CNPG clusters are Ready."""
    config.load_incluster_config()
    api = client.CustomObjectsApi()
    while True:
        ready = True
        for name in clusters:
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
            print("✅ All clusters ready")
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

def run_sql(host, db_name, admin_user, admin_password, statement, ignore_duplicate=True):
    """Run a SQL statement on the given host."""

    with psycopg2.connect(
        dbname=db_name,
        user=admin_user,
        password=admin_password,
        host=host,
        connect_timeout=3,
    ) as conn:
        conn.autocommit = False
        with conn.cursor() as cur:
            try:
                cur.execute(statement)
                conn.commit()
            except errors.DuplicateObject:
                if ignore_duplicate:
                    print(f"\tℹ️ Already exists on {host}")
                else:
                    raise
            except Exception as e:
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

    # Step 1: Discover CNPG clusters and wait for them to be ready
    time.sleep(10)  # wait a bit for k8s API to be ready
    print("🔎 Discovering CNPG clusters in namespace...")
    clusters = get_clusters(namespace)
    if not clusters:
        print("❌ No CNPG clusters found")
        sys.exit(1)

    print(f"🔎 Discovered clusters: {clusters}")
    wait_for_clusters(namespace, clusters)

    # Step 2: Wait for all clusters to accept connections
    for cluster in clusters:
        wait_ready(f"{cluster}-rw", db_name, admin_user, admin_password)

    # Step 3: Create pgedge user
    for cluster in clusters:
        stmt = f"""
            SELECT spock.repair_mode('True');
            CREATE ROLE {pgedge_user} WITH LOGIN SUPERUSER REPLICATION PASSWORD '{pgedge_password}';
        """
        run_sql(f"{cluster}-rw", db_name, admin_user, admin_password, stmt)
        print(f"👤 Created user {pgedge_user} on {cluster}")

    # Step 4: Create spock nodes
    for cluster in clusters:
        stmt = f"""
            SELECT spock.repair_mode('True');
            SELECT spock.node_create(
            node_name := '{cluster}',
            dsn := 'host={cluster}-rw.{namespace}.svc.cluster.local dbname={db_name} user={pgedge_user} password={pgedge_password} port=5432'
            )
            WHERE '{cluster}' NOT IN (SELECT node_name FROM spock.node);
        """
        run_sql(f"{cluster}-rw", db_name, admin_user, admin_password, stmt)
        print(f"🖥️ Created spock node {cluster} on {cluster}")

    forward_origins = "{}"
    replication_sets = "{default, default_insert_only, ddl_sql}"

    # Step 5: Wire subscriptions between every pair of spock nodes
    for src in clusters:
        for dst in clusters:
            if src != dst:
                other_dsn = f"host={dst}-rw.{namespace}.svc.cluster.local dbname={db_name} user={pgedge_user} password={pgedge_password} port=5432"
                sub_name = f"sub_{src}_{dst}".replace("-", "_")
                stmt = f"""
                    SELECT spock.sub_create(
                    subscription_name := '{sub_name}',
                    provider_dsn := '{other_dsn}',
                    replication_sets := '{replication_sets}',
                    forward_origins := '{forward_origins}',
                    synchronize_structure := 'false',
                    synchronize_data := 'false',
                    apply_delay := '0'
                ) WHERE '{sub_name}' NOT IN (SELECT sub_name FROM spock.subscription);
                """
                run_sql(f"{src}-rw", db_name, admin_user, admin_password, stmt)
                print(f"🔗 Created spock subscription {sub_name} on {src}")

    print("🎉 Spock nodes and subscriptions successfully initialized")

if __name__ == "__main__":
    main()