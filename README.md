# pgedge

TODO

![Version: 0.0.1](https://img.shields.io/badge/Version-0.0.1-informational?style=flat-square)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| annotations | object | `{}` |  |
| global.clusterDomain | string | `"cluster.local"` |  |
| labels | object | `{}` |  |
| pgEdge.appName | string | `"pgedge"` |  |
| pgEdge.dbSpec.dbName | string | `"defaultdb"` |  |
| pgEdge.dbSpec.nodes | list | `[]` |  |
| pgEdge.dbSpec.users[0].service | string | `"postgres"` |  |
| pgEdge.dbSpec.users[0].superuser | bool | `false` |  |
| pgEdge.dbSpec.users[0].type | string | `"application"` |  |
| pgEdge.dbSpec.users[0].username | string | `"app"` |  |
| pgEdge.dbSpec.users[1].service | string | `"postgres"` |  |
| pgEdge.dbSpec.users[1].superuser | bool | `true` |  |
| pgEdge.dbSpec.users[1].type | string | `"admin"` |  |
| pgEdge.dbSpec.users[1].username | string | `"admin"` |  |
| pgEdge.existingUsersSecret | string | `""` |  |
| pgEdge.imageTag | string | `"kube-testing"` |  |
| pgEdge.livenessProbe.enabled | bool | `true` |  |
| pgEdge.livenessProbe.failureThreshold | int | `6` |  |
| pgEdge.livenessProbe.initialDelaySeconds | int | `30` |  |
| pgEdge.livenessProbe.periodSeconds | int | `10` |  |
| pgEdge.livenessProbe.successThreshold | int | `1` |  |
| pgEdge.livenessProbe.timeoutSeconds | int | `5` |  |
| pgEdge.nodeCount | int | `2` |  |
| pgEdge.port | int | `5432` |  |
| pgEdge.readinessProbe.enabled | bool | `true` |  |
| pgEdge.readinessProbe.failureThreshold | int | `6` |  |
| pgEdge.readinessProbe.initialDelaySeconds | int | `5` |  |
| pgEdge.readinessProbe.periodSeconds | int | `5` |  |
| pgEdge.readinessProbe.successThreshold | int | `1` |  |
| pgEdge.readinessProbe.timeoutSeconds | int | `5` |  |
| pgEdge.resources | object | `{}` |  |
| pgEdge.terminationGracePeriodSeconds | int | `10` |  |
| service.annotations | object | `{}` |  |
| service.clusterIP | string | `""` |  |
| service.externalTrafficPolicy | string | `"Cluster"` |  |
| service.loadBalancerIP | string | `""` |  |
| service.loadBalancerSourceRanges | list | `[]` |  |
| service.name | string | `"pgedge"` |  |
| service.sessionAffinity | string | `"None"` |  |
| service.sessionAffinityConfig | object | `{}` |  |
| service.type | string | `"ClusterIP"` |  |
| storage.accessModes[0] | string | `"ReadWriteOnce"` |  |
| storage.annotations | object | `{}` |  |
| storage.className | string | `"standard"` |  |
| storage.labels | object | `{}` |  |
| storage.retentionPolicy.enabled | bool | `false` |  |
| storage.retentionPolicy.whenDeleted | string | `"Retain"` |  |
| storage.retentionPolicy.whenScaled | string | `"Retain"` |  |
| storage.selector | object | `{}` |  |
| storage.size | string | `"8Gi"` |  |

## Test configurations

This repository includes two example configurations: one that demonstrates this
chart in a single, standalone cluster and another that demonstrates a
multi-cluster installation.

## Pre-requisites

The example configurations require the following tools:

- A Docker host, e.g. Docker Desktop
- [`kind`](https://kind.sigs.k8s.io/)
    - Homebrew install command: `brew install kind`
- [Cilium CLI](https://github.com/cilium/cilium-cli)
    - Homebrew install command: `brew install cilium-cli`
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl)
    - Homebrew install command: `brew install kubectl`
- [`subctl`](https://submariner.io/operations/deployment/subctl/)
    - These examples assume v0.16.3
    - Install command: `curl -Ls https://get.submariner.io | VERSION=0.16.3 bash`

### Single cluster

The `single-*` `make` recipes create a local cluster with `kind` and install
the pgEdge chart in an intra-cluster HA configuration with two primary
instances.

```sh
# Create the cluster
make single-up

# Install pgEdge
make single-install

# Optional: follow the logs of an instance to watch initialization complete
kubectl --context kind-single logs --follow pgedge-0

# Create a test 'users' table on both instances
make single-create-test-table

# Enable replication on both instances
make single-enable-replication

# From here, any writes to the `users` table on either instance will be
# replicated to the other instance.

# You can use kubectl invoke psql on individual instances to demonstrate this
# behavior:
kubectl --context kind-single exec -it pgedge-0 -- psql -U app defaultdb
kubectl --context kind-single exec -it pgedge-1 -- psql -U app defaultdb

# You can scale the statefulset down and back up to simulate node failure and
# recovery
kubectl --context kind-single scale statefulset pgedge --replicas 1
kubectl --context kind-single scale statefulset pgedge --replicas 2

# Services running from within the cluster can connect to the pgedge service to
# load balance between the instances. You can run a postgres client in the
# cluster to try this out. First, get the password for the `app` user from the
# pgedge-users secret:
make print-secret

# Then, use the run-client command to start a postgresql container in the
# cluster.
make run-client

# Once you're presented with a bash shell, use the psql client to connect to the
# pgedge service:
PGPASSWORD=<password from secret> psql -h pgedge -U app defaultdb

# Afterwards, you can clean up by deleting the cluster
make single-down
```

### Multi cluster

The `multi-*` `make` recipes create two local clusters with `kind`. These
clusters are networked together using Cilium cluster mesh, and they use
Submariner to provide cross-cluster DNS.

```sh
# Create the clusters - NOTE: this takes a few minutes to run and you'll be
# prompted to select a gateway node for each cluster. Select the control plane
# node (the default selection) to continue.
make multi-up

# Install pgEdge
make multi-install

# Optional: follow the logs of an instance to watch initialization complete
kubectl --context kind-multi-iad logs --follow pgedge-iad-0

# Create a test 'users' table on all instances
make multi-create-test-table

# Enable replication on all instances
make multi-enable-replication

# From here, any writes to the `users` table on any instance will be
# replicated to the other instance.

# You can use kubectl invoke psql on individual instances to demonstrate this
# behavior:
kubectl --context kind-multi-iad exec -it pgedge-iad-0 -- psql -U app defaultdb
kubectl --context kind-multi-iad exec -it pgedge-iad-1 -- psql -U app defaultdb
kubectl --context kind-multi-sfo exec -it pgedge-sfo-0 -- psql -U app defaultdb
kubectl --context kind-multi-sfo exec -it pgedge-sfo-1 -- psql -U app defaultdb

# You can scale one of the statefulsets down and back up to simulate node
# failure and recovery
kubectl --context kind-multi-iad scale statefulset pgedge --replicas 0
kubectl --context kind-multi-iad scale statefulset pgedge --replicas 2

# Services running from within the cluster can connect to the pgedge service to
# load balance between the instances. This configuration uses the Cilium global
# service annotation to enable cross-cluster failover. You can run a postgres
# client in a cluster to try this out. First, get the password for the `app`
# user from the pgedge-users secret:
make print-secret

# Then, use the run-client command to start a postgresql container in the
# cluster.
make run-client

# Once you're presented with a bash shell, use the psql client to connect to the
# pgedge service:
PGPASSWORD=<password from secret> psql -h pgedge -U app defaultdb

# Afterwards, you can clean up by deleting both clusters
make multi-down
```

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.12.0](https://github.com/norwoodj/helm-docs/releases/v1.12.0)
