# Quickstart

This guide shows you how to install the pgEdge Helm chart
to deploy distributed, active-active PostgreSQL on
Kubernetes in under 5 minutes.

!!! tip "New to pgEdge on Kubernetes?"
    The [Guided Walkthrough](walkthrough.md) builds a cluster step-by-step, starting from a single primary and evolving to active-active replication. It's a great way to understand what each component does.

## Prerequisites

This guide deploys pgEdge's supported distribution of the
[CloudNativePG](https://cloudnative-pg.io/) operator, rebuilt
from upstream source.

For this, you need access to a Kubernetes cluster running a
[supported version](https://docs.pgedge.com/kubernetes/#version-support-matrix).

For local testing, see [Setting Up Local Kubernetes Environments](https://kubernetes.io/docs/setup/learning-environment/#setting-up-local-kubernetes-environments) in the official Kubernetes documentation.

Install the following tools to deploy and interact with
Kubernetes and CloudNativePG:

- [helm](https://helm.sh/) — the package manager for
  Kubernetes; used to install, upgrade, and manage applications
  via Helm charts.
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — the
  Kubernetes command-line tool; used to interact with and manage
  your clusters.
- [Krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/)
  — the kubectl plugin manager; used to install the cnpg plugin.

### Add the pgEdge Helm Repository

Use the `helm repo add` command to register the pgEdge Helm
Repository:

```bash
helm repo add pgedge https://pgedge.github.io/charts
helm repo update
```

### Install the cert-manager operator

Apply the cert-manager manifests and wait for the deployment
to become available:

```bash
kubectl apply -f \
  https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl wait --for=condition=Available deployment \
  --all -n cert-manager --timeout=120s
```

### Install the CloudNativePG operator

Install the operator from the pgEdge Helm repository:

```bash
helm install cnpg pgedge/cloudnative-pg \
  --namespace cnpg-system --create-namespace
kubectl wait --for=condition=Available deployment \
  -l app.kubernetes.io/name=cloudnative-pg \
  -n cnpg-system --timeout=120s
```

### Install the cnpg kubectl plugin

Add the pgEdge Krew index and install the plugin:

```bash
kubectl krew index add pgedge https://github.com/pgEdge/krew-index.git
kubectl krew install pgedge/cnpg
```

## Deploy

Each pgEdge node is a separate CloudNativePG Cluster that
participates in Spock active-active replication. Every node
accepts both reads and writes.

The following values file configures two nodes with Spock
enabled:

```yaml
pgEdge:
  appName: pgedge
  initSpock: true
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw
  clusterSpec:
    instances: 1
    storage:
      size: 1Gi
```

Save this as `values.yaml`, then install the chart:

```bash
helm install pgedge pgedge/pgedge -f values.yaml --wait --timeout 5m
```

!!! note
    Each node runs a single primary instance in this example.
    Increase `instances` to add streaming replicas with
    automatic failover within a node. See
    [Configuring Standby Instances](usage/standby.md)
    for details.

## Verify replication

The install created two independent PostgreSQL nodes —
`pgedge-n1` and `pgedge-n2` — each running as its own
CloudNativePG Cluster with a single primary instance.
Spock connects them with bidirectional logical replication;
a write to either node is automatically replicated to the
other.

Create a table on n1:

```bash
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "CREATE TABLE test (id int primary key, data text);"
```

Insert a row on n2:

```bash
kubectl cnpg psql pgedge-n2 -- -d app \
  -c "INSERT INTO test VALUES (1, 'written on n2');"
```

Read the row back from n1:

```bash
kubectl cnpg psql pgedge-n1 -- -d app \
  -c "SELECT * FROM test;"
```

If the row appears on n1, active-active replication is
working as expected.

## What's next

Now that you have a running cluster, explore the guides below
to connect your application(s), customize the deployment, and
scale to your desired topology:

- [Connecting To Postgres](usage/connecting.md)
- [Configuration](configuration.md)
- [Configuring Standby Instances](usage/standby.md)
- [Configuring Backups](usage/backups.md)
- [Adding Nodes](usage/adding_nodes.md)
- [Removing Nodes](usage/removing_nodes.md)

## Cleanup

Uninstall the pgEdge Helm chart:

```bash
helm uninstall pgedge
```

Helm removes all managed resources except secrets created by
cert-manager for client certificates. Delete those secrets
manually:

```bash
kubectl delete secret admin-client-cert app-client-cert \
  client-ca-key-pair pgedge-client-cert streaming-replica-client-cert
```

Remove the CloudNativePG operator:

```bash
helm uninstall cnpg -n cnpg-system
```

Remove cert-manager:

```bash
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

