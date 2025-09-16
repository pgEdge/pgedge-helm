# Example configurations

This directory contains an example configuration which demonstrates this chart in a single
Kubernetes cluster

- [Example configurations](#example-configurations)
  - [Pre-requisites](#pre-requisites)
  - [Single Kubernetes cluster](#single-kubernetes-cluster)
  - [Multiple Kubernetes clusters](#multiple-kubernetes-clusters)
  - [Testing pgEdge image updates](#testing-pgedge-image-updates)

## Pre-requisites

The example configurations require the following tools:

- A Docker host, e.g. Docker Desktop
- [`helm`](https://helm.sh/)
  - Homebrew install command: `brew install helm`
- [`kind`](https://kind.sigs.k8s.io/)
    - Homebrew install command: `brew install kind`
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl)
    - Homebrew install command: `brew install kubectl`
- ['kubectl CNPG plugin`](https://cloudnative-pg.io/documentation/current/kubectl-plugin/)
    - Homebrew install command `brew install kubectl-cnpg`

## Single Kubernetes cluster

The `single-*` `make` recipes create a local Kubernetes cluster with `kind` and install the pgEdge
chart with three nodes (n1, n2, n3). n1 is configured with 3 instances (1 primary, 2 standby), and n2/n3 are configured with just 1 primary.

```sh
# Create the cluster and install required operators
make single-up

# Install pgEdge helm chart
make single-install

# Optional: follow the logs of the spock-init job in a separate shell
kubectl --context kind-single logs --follow jobs/pgedge-init-spock

# You can use kubectl cnpg psql to connect to the app database as the postgres, app, or admin user with local trust
kubectl cnpg psql pgedge-n1 -- -U app app
kubectl cnpg psql pgedge-n2 -- -U app app
kubectl cnpg psql pgedge-n3 -- -U app app

# Connect to n1 to create a sample table and insert data
kubectl cnpg psql pgedge-n1 -- -U app -c "CREATE TABLE foo (id int PRIMARY KEY, value varchar); INSERT INTO foo VALUES (1, 'bar');"

# Connect to n2 to verify data has replicated successfully
kubectl cnpg psql pgedge-n2 -- -U app -c  "SELECT * from foo;"
kubectl cnpg psql pgedge-n3 -- -U app -c  "SELECT * from foo;"

# Fetch logs for a particular node with pretty formatting
kubectl cnpg logs cluster pgedge-n1 | kubectl cnpg logs pretty

# Identify the current primary and standby on n1
kubectl cnpg status pgedge-n1

# Promote a new primary on n1 (replacing the last parameter with a suitable standby candidate)
kubectl cnpg promote pgedge-n1 pgedge-n1-1

# Afterwards, you can clean up by deleting the Kubernetes cluster
make single-down
```