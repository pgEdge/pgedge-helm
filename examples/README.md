# Example configurations

This directory contains two example configurations: one that demonstrates this chart in a single
Kubernetes cluster and another that demonstrates a pgEdge cluster that spans multiple Kubernetes
clusters.

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

The [Multiple Kubernetes clusters] example has two additional requirements:

- [Cilium CLI](https://github.com/cilium/cilium-cli)
    - Homebrew install command: `brew install cilium-cli`
- [`subctl`](https://submariner.io/operations/deployment/subctl/)
    - These examples assume v0.16.3
    - Install command: `curl -Ls https://get.submariner.io | VERSION=0.16.3 bash`

## Single Kubernetes cluster

The `single-*` `make` recipes create a local Kubernetes cluster with `kind` and install the pgEdge
chart in an intra-cluster HA configuration with two primary instances.

```sh
# Create the cluster
make single-up

# Install pgEdge
make single-install

# Optional: follow the logs of an instance to watch initialization complete
kubectl --context kind-single logs --follow pgedge-0

# Create a test 'users' table on both instances
make single-create-test-table

# Optional: If you are not using AutoDDL, enable replication on both instances for this table
# If you are using AutoDDL (the default), this will be handled automatically
make single-enable-replication

# From here, any writes to the `users` table on either instance will be replicated to the other
# instance.

# You can use kubectl invoke psql on individual instances to demonstrate this behavior. 
# If you are using AutoDDL, you can create tables and insert data to test the database
kubectl --context kind-single exec -it pgedge-0 -- psql -U app defaultdb
kubectl --context kind-single exec -it pgedge-1 -- psql -U app defaultdb


# You can scale the statefulset down and back up to simulate node failure and
# recovery
kubectl --context kind-single scale statefulset pgedge --replicas 1
kubectl --context kind-single scale statefulset pgedge --replicas 2

# Services running from within the Kubernetes cluster can connect to the 'pgedge' service to load
# balance between the pgEdge instances. You can run a postgres client in the cluster to try this
# out. First, get the password for the `app` user from the pgedge-users secret:
make print-secret

# Then, use the run-client command to start a postgresql container in the current Kubernetes
# context.
make run-client

# Once you're presented with a bash shell, use the psql client to connect to the pgedge service:
PGPASSWORD=<password from secret> psql -h pgedge -U app defaultdb

# Afterwards, you can clean up by deleting the Kubernetes cluster
make single-down
```

## Multiple Kubernetes clusters

The `multi-*` `make` recipes create two local Kubernetes clusters with `kind` and installs a single
pgEdge cluster that spans both Kubernetes clusters. The Kubernetes clusters are networked together
using Cilium cluster mesh, and they use Submariner to provide cross-cluster DNS.

```sh
# Create the Kubernetes clusters - NOTE: this takes a few minutes to run and you'll be prompted to
# select a gateway node for each cluster. Select the control plane node (the default selection) to
# continue.
make multi-up

# Install pgEdge
make multi-install

# Optional: follow the logs of an instance to watch initialization complete
kubectl --context kind-multi-iad logs --follow pgedge-iad-0

# Create a test 'users' table on all instances
make multi-create-test-table

# Optional: If you are not using AutoDDL, enable replication on both instances for this table
# If you are using AutoDDL (the default), this will be handled automatically
make multi-enable-replication

# From here, any writes to the `users` table on any instance will be replicated to the other
# instance.

# You can use kubectl invoke psql on individual instances to demonstrate this behavior:
kubectl --context kind-multi-iad exec -it pgedge-iad-0 -- psql -U app defaultdb
kubectl --context kind-multi-iad exec -it pgedge-iad-1 -- psql -U app defaultdb
kubectl --context kind-multi-sfo exec -it pgedge-sfo-0 -- psql -U app defaultdb
kubectl --context kind-multi-sfo exec -it pgedge-sfo-1 -- psql -U app defaultdb

# You can scale one of the statefulsets down and back up to simulate node failure and recovery
kubectl --context kind-multi-iad scale statefulset pgedge --replicas 0
kubectl --context kind-multi-iad scale statefulset pgedge --replicas 2

# Services running from within one of the Kubernetes clusters can connect to the 'pgedge' service to
# load balance across all pgEdge instances. This configuration uses the Cilium global service
# annotation to enable cross Kubernetes-cluster failover. You can run a postgres client in one of
# the Kubernetes clusters to try this out. First, get the password for the `app` user from the
# pgedge-users secret:
make print-secret

# Then, use the run-client command to start a postgresql container in the current Kubernetes
# context.
make run-client

# Once you're presented with a bash shell, use the psql client to connect to the pgedge service:
PGPASSWORD=<password from secret> psql -h pgedge -U app defaultdb

# Afterwards, you can clean up by deleting both Kubernetes clusters
make multi-down
```

## Testing pgEdge image updates

The default image tag is specified in the top-level [values.yaml](../values.yaml) file. You can do
the following to use the the example configurations to test image updates:

* Build and tag the image if it doesn't already exist, e.g. `pgedge/pgedge:kube-testing`
* Update the `pgEdge.imageTag` value in the top-level `values.yaml` file
* Start one of the examples with either `make single-up` or `make multi-up`
* Run the corresponding `*-load-image` recipe, e.g. `make single-load-image`
  * This copies the image from your host machine into the `kind` cluster(s)
* Install and test pgEdge using the steps from the previous sections

After you've completed your tests, remember to update `pgedge.imageTag` to one that's been pushed to
Docker Hub.

## Running ACE (the Active Consistency Engine)

A sample config for running ACE as a temporary Pod is [included](/ace/ace.yaml) in this repository. This can be useful to analyze the state of your database across nodes. You can learn more about ACE in the [pgEdge documentation](https://docs.pgedge.com/platform/ace).

This example assumes that the Helm chart is already deployed and running, and leverages the deployed ConfigMap and Secret to setup the required configuration for ACE.

You can run the Pod using kubectl:

``` sh
kubectl apply -f examples/ace/ace.yaml
```

From there, ACE can be invoked by shelling into the Pod and invoking ACE.

```sh
kubectl exec -it ace -- /bin/bash
./ace schema-diff defaultdb public
```
