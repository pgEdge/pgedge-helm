# pgEdge Quickstart

Deploy distributed, active-active PostgreSQL on Kubernetes in under
5 minutes.

> Want a guided walkthrough instead?
> [Codespaces](https://codespaces.new/pgEdge/pgedge-helm?devcontainer_path=.devcontainer/demo/devcontainer.json) (VS Code)
> | [Local guide](../examples/try-locally/) (Docker + kind)

This guide uses pgEdge's curated distribution of the
[CloudNativePG](https://cloudnative-pg.io/) operator — rebuilt from
upstream source and published to the pgEdge registry
(`ghcr.io/pgedge/cloudnative-pg`). The operator, Helm charts, kubectl
plugin, and backup plugins are all installable from the pgEdge Helm
repo. See
[pgEdge/pgedge-cnpg-dist](https://github.com/pgEdge/pgedge-cnpg-dist)
for details.

The pgEdge distribution is not affiliated with, endorsed by, or
sponsored by the CloudNativePG project or the Cloud Native Computing
Foundation.

## Prerequisites

A Kubernetes cluster with kubectl and Helm installed.

### Add the pgEdge Helm repo

```bash
helm repo add pgedge https://pgedge.github.io/charts
helm repo update
```

### Install cert-manager

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

### Install the pgEdge CloudNativePG operator

```bash
helm install cnpg pgedge/cloudnative-pg \
  --namespace cnpg-system --create-namespace
kubectl wait --for=condition=Available deployment \
  -l app.kubernetes.io/name=cloudnative-pg \
  -n cnpg-system --timeout=120s
```

### Install the cnpg kubectl plugin

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
curl -sSfL \
  "https://github.com/pgEdge/pgedge-cnpg-dist/releases/download/v1.28.0/kubectl-cnpg-${OS}-${ARCH}.tar.gz" \
  -o /tmp/cnpg.tar.gz
sudo tar xzf /tmp/cnpg.tar.gz -C /usr/local/bin
```

## Deploy

Install a 2-node multi-master cluster. Both nodes accept reads and
writes via Spock active-active replication:

```bash
helm install pgedge pgedge/pgedge \
  -f examples/try-locally/values/quickstart.yaml
```

Wait for both nodes to be ready:

```bash
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n1 --timeout=300s
kubectl wait --for=condition=Ready pod \
  -l cnpg.io/cluster=pgedge-n2 --timeout=300s
```

Check Spock subscriptions are active:

```bash
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT subscription_name, status FROM spock.sub_show_status();"
```

## Verify replication

Create a table on n1, insert on n2, read back on n1:

```bash
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "CREATE TABLE test (id int primary key, data text);"
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "INSERT INTO test VALUES (1, 'written on n2');"
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM test;"
```

If you see the row on n1, active-active replication is working.

## What's next

### Example configurations

The [`examples/configs/single/`](../examples/configs/single/) directory
has a ready-to-deploy values file for a single-region deployment with
3 nodes and 3 instances. More topology examples are coming soon.

### Step-by-step walkthrough

Want to see how the architecture evolves from a single primary through
HA to multi-master? See
[examples/try-locally/WALKTHROUGH.md](../examples/try-locally/WALKTHROUGH.md)
for the progressive guide.

### Helm chart documentation

| Topic | Description |
|---|---|
| [Configuration](configuration.md) | Full values reference, cluster spec options |
| [Installation](install.md) | Detailed installation and prerequisites |
| [Upgrading Postgres](usage/postgres_upgrades.md) | Minor/major version upgrades, image pinning |
| [Adding Nodes](usage/adding_nodes.md) | Scale out with Spock or CNPG bootstrap |
| [Backups](usage/backups.md) | Barman Cloud backups to S3, Azure, GCS |
| [Monitoring](usage/monitoring.md) | Health checks and observability |
| [Standby Instances](usage/standby.md) | Read replicas with automatic failover |
| [Multi-cluster](multicluster.md) | Cross-region deployments, external nodes |
| [Security](security.md) | Pod security standards, TLS certificates |

- [pgEdge Documentation](https://docs.pgedge.com) — Spock replication,
  conflict resolution, tuning
- [pgEdge Cloud](https://www.pgedge.com) — managed distributed
  PostgreSQL

## Cleanup

```bash
helm uninstall pgedge
```
