---
cwd: ../
---
# Guided Walkthrough

This walkthrough progressively builds an active-active
PostgreSQL deployment using pgEdge Helm on Kubernetes.
Instead of deploying everything at once, the architecture
evolves step-by-step:

| Step | What you'll do |
|------|---------------|
| Set Up Kubernetes | Install the operators that manage PostgreSQL on Kubernetes |
| Deploy a Single Primary Instance | Deploy one pgEdge node with a single Postgres instance |
| Add Standby Instances | Add a synchronous standby instance for high availability |
| Add a Second Node | Add a second pgEdge node with Spock active-active replication |
| Verify Replication | Write data on one node, read it on the other |

Each deployment step uses Helm (`install` or `upgrade`),
so the cluster evolves in real time.

!!! tip "Run the commands as you read"
    Every code block in this walkthrough is executable.
    Open the walkthrough in
    [GitHub Codespaces](https://github.com/codespaces/new?repo=pgEdge/pgedge-helm&devcontainer_path=.devcontainer/walkthrough/devcontainer.json)
    for a ready-to-go environment, or install the
    [Runme extension](https://marketplace.visualstudio.com/items?itemName=stateful.runme)
    in VS Code to run commands directly from the markdown.


## Step 1: Set Up Kubernetes

Before deploying pgEdge Helm, a Kubernetes cluster and two
operators are required:

- cert-manager — handles TLS certificates so that database
  nodes communicate securely.
- CloudNativePG (CNPG) — manages PostgreSQL clusters as native
  Kubernetes resources; handles pod creation, failover,
  replication, and backups.

### Create a local cluster

The following command downloads the walkthrough files, sets up
tools, and creates a local cluster:

```shell
curl -fsSL \
  https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/walkthrough/install.sh \
  | bash
```

!!! note "Codespaces users"
    The cluster is already running — skip to
    [Install cert-manager](#install-cert-manager). Or run
    `bash examples/walkthrough/guide.sh` for an interactive
    terminal experience instead of this walkthrough.

### Install cert-manager

The following command installs cert-manager from the
latest upstream release:

```shell
kubectl apply -f \
  https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

Wait for all three cert-manager pods to become `Running`:

```shell
kubectl wait --for=condition=Available deployment \
  --all -n cert-manager --timeout=120s
```

### Add the pgEdge Helm repository

The following commands add the pgEdge chart repository
and update the local cache:

```shell
helm repo add pgedge \
  https://pgedge.github.io/charts --force-update
helm repo update
```

### Install the CloudNativePG operator

The following command installs the pgEdge CloudNativePG
operator into a dedicated namespace:

```shell
helm install cnpg pgedge/cloudnative-pg \
  --namespace cnpg-system --create-namespace
```

Wait for the operator to be ready:

```shell
kubectl wait --for=condition=Available deployment \
  -l app.kubernetes.io/name=cloudnative-pg \
  -n cnpg-system --timeout=120s
```

### Verify the environment

The cnpg kubectl plugin provides the `cnpg status` and
`cnpg psql` commands used throughout this walkthrough.
Verify the plugin is installed:

```shell
kubectl cnpg version
```

Confirm the pgEdge Helm chart is available:

```shell
helm search repo pgedge
```

The `pgedge/pgedge` chart should appear in the results.


## Step 2: Deploy a Single Primary Instance

This step deploys the simplest possible configuration:
one pgEdge node running a single PostgreSQL instance.

### Review the values file

The values file defines one node (`n1`) with a single instance:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec:
        instances: 1
  clusterSpec:
    storage:
      size: 1Gi
```

### Install the chart

The following command installs the pgEdge Helm chart with
the single-primary values file:

```shell
helm install pgedge pgedge/pgedge \
  -f examples/walkthrough/values/step1-single-primary.yaml
```

Wait for the instance to be ready:

```shell
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n1 --timeout=180s
```

### Check the cluster status

The following command shows instance count, replication
state, and overall health:

```shell
kubectl cnpg status pgedge-n1
```

Expected output:

- Instances: 1
- Ready instances: 1
- Status: Cluster in healthy state

### Connect and verify

The pgEdge Helm chart creates a database called `app` with the
Spock extension pre-installed. The following command confirms
the database is accessible:

```shell
kubectl cnpg psql pgedge-n1 -- \
  -d app -c "SELECT version();"
```

### Load some data

Create a `cities` table and insert a few rows. This data
will carry forward through the rest of the walkthrough:

```shell
kubectl cnpg psql pgedge-n1 -- -d app -c "
CREATE TABLE cities (
  id INT PRIMARY KEY,
  name TEXT NOT NULL,
  country TEXT NOT NULL
);

INSERT INTO cities (id, name, country) VALUES
  (1, 'New York', 'USA'),
  (2, 'London', 'UK'),
  (3, 'Tokyo', 'Japan');"
```

Confirm the data is there:

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM cities;"
```

A single PostgreSQL primary is now running with data,
managed by the CloudNativePG operator and deployed via
the pgEdge Helm chart.


## Step 3: Add Standby Instances

This step upgrades the deployment to add a synchronous
standby instance. Standby instances provide high availability —
if the primary fails, a standby takes over with zero
data loss.

### What's changing

The key differences from step 2:

- `instances: 1` becomes `instances: 2` — adds a standby
  instance to node n1.
- Synchronous replication is configured with
  `dataDurability: required` — every write is confirmed on
  both instances before the transaction completes.

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec:
        instances: 2
        postgresql:
          synchronous:
            method: any
            number: 1
            dataDurability: required
  clusterSpec:
    storage:
      size: 1Gi
```

### Upgrade the release

This is a `helm upgrade`, not a new install. The existing
primary stays running while the standby instance is added:

```shell
helm upgrade pgedge pgedge/pgedge \
  -f examples/walkthrough/values/step2-with-replicas.yaml
```

Wait for both instances to be ready:

```shell
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n1 --timeout=180s
```

### Check the cluster status

Two instances should now be visible — one primary and one
standby with the `(sync)` role:

```shell
kubectl cnpg status pgedge-n1
```

### Verify replication is working

The following query shows the replication connection from
the primary's perspective. Look for
`sync_state = sync` or `quorum`:

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT client_addr, state, sync_state FROM pg_stat_replication;"
```

The standby instance receives all changes synchronously.
Every committed write is guaranteed to exist on both
instances before the transaction completes.


## Step 4: Add a Second Node

This is where pgEdge shines. This step adds a second
pgEdge node (`n2`) with Spock active-active replication.
Both nodes accept writes, and changes replicate
bidirectionally.

Unlike the standby instance in step 3 (which exists for
failover), both n1 and n2 independently accept reads and
writes.

### What's changing

A second node `n2` is added to the `nodes` list. The key
setting is `bootstrap.mode: spock`, which tells the chart
to clone data from n1 and set up Spock logical replication
between the two nodes:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec:
        instances: 2
        postgresql:
          synchronous:
            method: any
            number: 1
            dataDurability: required
    - name: n2
      hostname: pgedge-n2-rw
      clusterSpec:
        instances: 1
      bootstrap:
        mode: spock
        sourceNode: n1
  clusterSpec:
    storage:
      size: 1Gi
```

### Upgrade the release

The following command adds the second node to the release:

```shell
helm upgrade pgedge pgedge/pgedge \
  -f examples/walkthrough/values/step3-multi-master.yaml
```

The CNPG operator creates a new cluster for n2, and the
pgEdge init-spock job wires up Spock subscriptions between
the nodes. This step takes longer than previous upgrades.

Wait for both clusters to be ready:

```shell
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n1 --timeout=180s
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n2 --timeout=180s
```

### Check both clusters

The following commands display the status of each node:

```shell
kubectl cnpg status pgedge-n1
```

```shell
kubectl cnpg status pgedge-n2
```

### Verify Spock replication

Each node subscribes to the other — that is what makes the
cluster active-active. Both should show subscriptions with
status `replicating`:

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM spock.sub_show_status();"
```

```shell
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "SELECT * FROM spock.sub_show_status();"
```

### Verify data was bootstrapped

The cities table and its data were created on n1 in step 2.
The bootstrap process cloned everything to n2. Query n2 to
confirm:

```shell
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "SELECT * FROM cities;"
```

All 3 cities should be visible on n2 — without any manual
data loading.

Both nodes now accept reads and writes, with changes
replicating automatically via Spock.


## Step 5: Verify Replication

The data on n2 was bootstrapped from n1, but active-active
means both nodes can accept writes going forward. This step
proves bidirectional replication by writing new data on each
node and reading it on the other.

### Write on n2

The following command inserts two new cities on n2:

```shell
kubectl cnpg psql pgedge-n2 -- -d app -c "
INSERT INTO cities (id, name, country) VALUES
  (4, 'Sydney', 'Australia'),
  (5, 'Berlin', 'Germany');"
```

### Read back on n1

All 5 rows should be present — 3 written on n1 in step 2,
and 2 replicated from n2:

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM cities ORDER BY id;"
```

### Write on n1, read on n2

The following commands insert a city on n1 and verify
the row appears on n2:

```shell
kubectl cnpg psql pgedge-n1 -- -d app -c "
INSERT INTO cities (id, name, country) VALUES
  (6, 'Paris', 'France');"
```

```shell
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "SELECT * FROM cities ORDER BY id;"
```

All 6 cities appear on both nodes, confirming bidirectional
active-active replication.


## Explore Further

The active-active deployment is complete. Here are some
things to try.

### Load more data

The following command creates a `products` table and inserts
sample rows on n1:

```shell
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

Verify the data replicated to n2:

```shell
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "SELECT * FROM products;"
```

### Inspect Spock configuration

The following queries show how Spock manages replication
sets and nodes:

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM spock.replication_set;"
```

```shell
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM spock.node;"
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


## Cleanup

Uninstall the pgEdge Helm release:

```shell
helm uninstall pgedge
```

If using a local kind cluster, delete the cluster. This
removes everything including the operators:

```shell
kind delete cluster --name pgedge-demo
```

If using an existing cluster, remove the operators
separately:

```shell
helm uninstall cnpg --namespace cnpg-system
kubectl delete -f \
  https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```


## What's Next

Explore the guides below to connect your application,
customize the deployment, and learn more about pgEdge
on Kubernetes:

- [Connecting To Postgres](usage/connecting.md)
- [Configuration](configuration.md)
- [Configuring Standby Instances](usage/standby.md)
- [Configuring Backups](usage/backups.md)
- [Adding Nodes](usage/adding_nodes.md)
- [Removing Nodes](usage/removing_nodes.md)
- [pgEdge Kubernetes Documentation](https://docs.pgedge.com/kubernetes/) — Installation, upgrades, and support.
- [Spock Documentation](https://docs.pgedge.com/spock/spock5/) — Active-active replication, conflict resolution, and more.
