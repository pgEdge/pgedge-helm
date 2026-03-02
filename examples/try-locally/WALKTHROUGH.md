# pgEdge Distributed Postgres on Kubernetes

In this walkthrough you'll progressively build a **distributed PostgreSQL cluster** using pgEdge on Kubernetes. Instead of deploying everything at once, you'll evolve the architecture step-by-step:

| Step | What you'll do |
|------|---------------|
| **Set Up Cluster** | Install the operators that manage PostgreSQL on Kubernetes |
| **Single Primary** | Deploy one pgEdge node with a single Postgres instance |
| **HA with Replicas** | Add a synchronous read replica for high availability |
| **Multi-Master** | Add a second pgEdge node with Spock active-active replication |
| **Prove It Works** | Write data on one node, read it on the other |

Each step is a `helm upgrade`, so you'll see the cluster evolve in real time.

Click the **Run** button on any code block to execute it directly in the terminal.

> **Prefer a guided terminal experience?** Instead of clicking through
> Runme, run the interactive guide script:
>
> ```bash
> ./guide.sh
> ```
>
> It walks you through the same steps with explanations and prompts.

---

## Step 1: Set Up the Cluster

Before deploying pgEdge, we need two Kubernetes operators:

- **cert-manager** — handles TLS certificates so that database nodes communicate securely
- **CloudNativePG (CNPG)** — manages PostgreSQL clusters as native Kubernetes resources, handling pod creation, failover, replication, and backups

The setup script creates a local kind cluster (if needed) and installs both operators. This takes about 2 minutes:

```bash
bash scripts/setup-cluster.sh
```

### Verify the environment

Check that the Kubernetes cluster is running:

```bash
kubectl get nodes
```

You should see one node in `Ready` state.

Check the CNPG operator is running — this is what will manage our PostgreSQL pods:

```bash
kubectl get deployment -n cnpg-system
```

Check cert-manager — all three pods should be `Running`:

```bash
kubectl get pods -n cert-manager
```

Check the cnpg kubectl plugin — this gives us `cnpg status` and `cnpg psql` commands:

```bash
kubectl cnpg version
```

Check the pgEdge Helm chart is available:

```bash
helm search repo pgedge
```

You should see the `pgedge/pgedge` chart listed.

---

## Step 2: Deploy a Single Primary

Let's start with the simplest possible deployment: one pgEdge node running a single PostgreSQL instance.

### Review the values file

The values file defines just one node (`n1`) with 1 instance:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
  clusterSpec:
    instances: 1
    storage:
      size: 1Gi
```

### Install the chart

```bash
helm install pgedge pgedge/pgedge -f values/step1-single-primary.yaml
```

The CNPG operator is now creating a PostgreSQL pod. Wait for it to be ready:

```bash
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s
```

### Check the cluster status

This shows instance count, replication state, and overall health:

```bash
kubectl cnpg status pgedge-n1
```

You should see:
- **Instances:** 1
- **Ready instances:** 1
- **Status:** Cluster in healthy state

### Connect and verify

The pgEdge chart creates a database called `app` with the Spock extension pre-installed. Let's confirm it's working:

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT version();"
```

You now have a single PostgreSQL primary running in Kubernetes, managed by the CNPG operator and deployed via the pgEdge Helm chart.

---

## Step 3: Scale with Read Replicas

Now let's upgrade the deployment to add a **synchronous read replica**. This gives you high availability — if the primary fails, the replica takes over with **zero data loss**.

### What's changing

The key differences from step 2:

- `instances: 1` becomes `instances: 2` — adds a replica to node n1
- Synchronous replication is configured with `dataDurability: required` — every write is confirmed on both instances before the transaction completes

```yaml
nodes:
  - name: n1
    hostname: pgedge-n1-rw
    clusterSpec:
      instances: 2           # was 1
      postgresql:
        synchronous:
          method: any
          number: 1
          dataDurability: required   # zero data loss
```

### Upgrade the release

This is a `helm upgrade`, not a new install. The existing primary stays running while the replica is added:

```bash
helm upgrade pgedge pgedge/pgedge -f values/step2-with-replicas.yaml
```

Wait for both pods to be ready:

```bash
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s
```

### Check the cluster status

You should now see 2 instances — one primary and one replica with the `(sync)` role:

```bash
kubectl cnpg status pgedge-n1
```

### Verify replication is working

This query shows the replication connection from the primary's perspective. Look for `sync_state = sync` or `quorum`:

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT client_addr, state, sync_state FROM pg_stat_replication;"
```

The replica is receiving all changes synchronously — every committed write is guaranteed to be on both instances before the transaction completes.

---

## Step 4: Go Multi-Master

This is where pgEdge shines. You'll add a **second pgEdge node** (`n2`) with **Spock active-active replication**. Both nodes will accept writes, and changes replicate bidirectionally.

Unlike the read replica in the previous step (which only accepts reads), both n1 and n2 are **full read-write nodes**.

### What's changing

A second node `n2` is added to the `nodes` list. The key setting is `bootstrap.mode: spock`, which tells the chart to automatically set up Spock logical replication between n1 and n2:

```yaml
nodes:
  - name: n1              # existing — keeps its replica
    hostname: pgedge-n1-rw
    clusterSpec:
      instances: 2
      # ... synchronous config unchanged
  - name: n2              # new — bootstraps from n1
    hostname: pgedge-n2-rw
    bootstrap:
      mode: spock
      sourceNode: n1
```

### Upgrade the release

```bash
helm upgrade pgedge pgedge/pgedge -f values/step3-multi-master.yaml
```

This takes a bit longer — the CNPG operator creates a new cluster for n2, and the pgEdge init-spock job wires up Spock subscriptions between the nodes.

Wait for both clusters to be ready:

```bash
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n2 --timeout=180s
```

### Check both clusters

```bash
kubectl cnpg status pgedge-n1
```

```bash
kubectl cnpg status pgedge-n2
```

### Verify Spock replication

Each node subscribes to the other — that's what makes it active-active. Both should show subscriptions with status `replicating`:

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT * FROM spock.sub_show_status();"
```

```bash
kubectl cnpg psql pgedge-n2 -- -d app -c "SELECT * FROM spock.sub_show_status();"
```

You now have a **distributed, active-active PostgreSQL cluster** on Kubernetes. Both nodes accept reads and writes, with changes replicating automatically via Spock.

---

## Step 5: Prove Replication

Let's verify that active-active replication is working by writing data on one node and reading it on the other — in both directions.

### Create a table on n1

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "
CREATE TABLE cities (
  id INT PRIMARY KEY,
  name TEXT NOT NULL,
  country TEXT NOT NULL
);"
```

### Insert data on n1

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "
INSERT INTO cities (id, name, country) VALUES
  (1, 'New York', 'USA'),
  (2, 'London', 'UK'),
  (3, 'Tokyo', 'Japan');"
```

### Read on n2

These rows were written on n1 but are already replicated to n2 via Spock:

```bash
kubectl cnpg psql pgedge-n2 -- -d app -c "SELECT * FROM cities;"
```

You should see all 3 cities.

### Now write on n2

This is the active-active part — n2 can accept writes too:

```bash
kubectl cnpg psql pgedge-n2 -- -d app -c "
INSERT INTO cities (id, name, country) VALUES
  (4, 'Sydney', 'Australia'),
  (5, 'Berlin', 'Germany');"
```

### Read back on n1

All 5 rows should be here — 3 written locally on n1 and 2 replicated from n2:

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT * FROM cities ORDER BY id;"
```

**All 5 cities on both nodes — bidirectional active-active replication confirmed!**

---

## Explore Further

You have a working distributed PostgreSQL cluster. Here are some things to try.

### Load more data

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "
CREATE TABLE products (
  id INT PRIMARY KEY,
  name TEXT NOT NULL,
  category TEXT,
  price DECIMAL(10,2)
);

INSERT INTO products (id, name, category, price) VALUES
  (1, 'Chai', 'Beverages', 18.00),
  (2, 'Chang', 'Beverages', 19.00),
  (3, 'Aniseed Syrup', 'Condiments', 10.00),
  (4, 'Cajun Seasoning', 'Condiments', 22.00),
  (5, 'Olive Oil', 'Condiments', 21.35);"
```

Verify it replicated:

```bash
kubectl cnpg psql pgedge-n2 -- -d app -c "SELECT * FROM products;"
```

### Inspect Spock configuration

See how Spock manages replication sets and nodes:

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT * FROM spock.replication_set;"
```

```bash
kubectl cnpg psql pgedge-n1 -- -d app -c "SELECT * FROM spock.node;"
```

### Useful commands

| Command | What it does |
|---------|-------------|
| `kubectl cnpg status pgedge-n1` | Show n1 cluster health and replication |
| `kubectl cnpg status pgedge-n2` | Show n2 cluster health and replication |
| `kubectl cnpg psql pgedge-n1 -- -d app` | Open a psql shell to n1 |
| `kubectl cnpg psql pgedge-n2 -- -d app` | Open a psql shell to n2 |
| `kubectl get pods -o wide` | See all pods with node placement |
| `helm get values pgedge` | See current Helm values |

---

## Cleanup

To tear down the demo environment:

```bash
helm uninstall pgedge
```

```bash
kind delete cluster --name pgedge-demo
```

---

## Learn More

- [pgEdge Helm Chart](https://github.com/pgedge/pgedge-helm) — Full chart documentation
- [pgEdge Documentation](https://docs.pgedge.com) — Spock replication, conflict resolution, and more
- [pgEdge Cloud](https://www.pgedge.com) — Managed distributed PostgreSQL
