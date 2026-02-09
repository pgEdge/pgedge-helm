# Quickstart

This quickstart guide shows you how to install the pgEdge Helm chart to deploy Postgres in a Kubernetes cluster. You can choose between a distributed setup with three nodes or a single-node configuration.

The guide covers basic usage of the Helm chart and its features, using either an existing Kubernetes cluster or a local cluster on your laptop.

## Prerequisites

- Access to a Kubernetes cluster:
    - To run a cluster locally, we recommend using [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
    - To use `kind`, you can initialize a local Kubernetes cluster using this command:
        ``` sh
        kind create cluster --config examples/configs/single/kind.yaml
        ```
- The following tools must be installed on your laptop to deploy and interact with Kubernetes and CloudNativePG:
    - [helm](https://helm.sh/docs/intro/install/): The package manager for Kubernetes, used to install, upgrade, and manage Kubernetes applications via Helm charts.
    - [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl): The Kubernetes command-line tool, used to interact with and manage your Kubernetes clusters.
    - [kubectl cnpg plugin](https://github.com/pgEdge/pgedge-cnpg-dist/releases?q=kubectl-cnpg&expanded=true): A plugin for `kubectl` that provides additional commands for managing CloudNativePG clusters. Download the appropriate binary for your platform from the pgEdge distribution releases.

## Installation

1. Add the pgEdge Helm repository:

    ```sh
    helm repo add pgedge https://pgedge.github.io/charts
    helm repo update
    ```

2. Set your kubectl context and namespace to ensure you will deploy the chart to the correct cluster. For example:

    ```sh
    kubectl config use-context <cluster-context> --namespace <desired-namespace>
    ```

    If you are using `kind`, your context will be called `kind-single`.

3. Install chart dependencies.

    pgEdge Helm requires the `CloudNativePG` and `cert-manager` operators to be installed into your cluster:

    ```shell
    # Install CloudNativePG operator
    helm install cnpg pgedge/cloudnative-pg \
      --namespace cnpg-system \
      --create-namespace

    # Install cert-manager
    kubectl apply -f \
      https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml

    kubectl wait --for=condition=Available deployment \
      -n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=120s
    ```

4. Customize your chart configuration (optional).

    Create a `values.yaml` file with your configuration. For a distributed setup with three nodes:

    ```yaml
    pgEdge:
      appName: pgedge
      nodes:
        - name: n1
          hostname: pgedge-n1-rw
          clusterSpec:
            instances: 3
            postgresql:
              synchronous:
                method: any
                number: 1
                dataDurability: required
        - name: n2
          hostname: pgedge-n2-rw
        - name: n3
          hostname: pgedge-n3-rw
      clusterSpec:
        storage:
          size: 1Gi
    ```

    For a single region deployment with just one node:

    ```yaml
    pgEdge:
      appName: pgedge
      nodes:
        - name: n1
          hostname: pgedge-n1-rw
          clusterSpec:
            instances: 3
            postgresql:
              synchronous:
                method: any
                number: 1
                dataDurability: required
      clusterSpec:
        storage:
          size: 1Gi
    ```

    If you want to change your configuration later to use a distributed architecture, see [Adding Nodes](usage/adding_nodes.md).

    You can also reference the example configuration at [examples/configs/single/values.yaml](https://github.com/pgEdge/pgedge-helm/blob/main/examples/configs/single/values.yaml).

5. Install the chart.

    **From pgEdge Helm Repository (recommended):**

    ```sh
    helm install pgedge pgedge/pgedge \
      --values values.yaml \
      --wait
    ```

    **From local chart:**

    Alternatively, download the latest release from [pgEdge Helm Releases](https://github.com/pgEdge/pgedge-helm/releases/), extract it, and install from the local directory:

    ```sh
    helm install pgedge ./ \
      --values values.yaml \
      --wait
    ```

    Once the deployment is complete, you should see a confirmation message, and your database is ready to use:

    ```sh
    NAME: pgedge
    LAST DEPLOYED: Thu Oct 30 10:41:07 2025
    NAMESPACE: default
    STATUS: deployed
    REVISION: 1
    TEST SUITE: None
    ```

## Connecting to each Database Instance

If you have the `kubectl cnpg` plugin installed on your machine, you can
access each instance as follows:

```sh
# The 'n1' node's primary instance
kubectl cnpg psql pgedge-n1 -- -U app app

# The first available read replica for the  'n1' node
kubectl cnpg psql pgedge-n1 --replica -- -U app app

# The 'n2' node's primary instance, if deploying a distributed database
kubectl cnpg psql pgedge-n2 -- -U app app

# The 'n3' node's primary instance, if deploying a distributed database
kubectl cnpg psql pgedge-n3 -- -U app app
```

## Trying out Replication

1. Create a table on the first node:

    ```sh
    kubectl cnpg psql pgedge-n1 -- -U app app -c "create table example (id int primary key, data text);"
    ```

2. Insert a row into our new table on the second node:

    ```sh
    kubectl cnpg psql pgedge-n2 -- -U app app -c "insert into example (id, data) values (1, 'Hello, pgEdge!');"
    ```

3. See that the new row has replicated back to the first node:

    ```sh
    kubectl cnpg psql pgedge-n1 -- -U app app -c "select * from example;"
    ```

## Loading the Northwind Sample dataset

The Northwind sample dataset is a PostgreSQL database dump that you can use to
try replication with a more realistic database. To load the Northwind dataset
into your pgEdge database, run:

```sh
curl https://downloads.pgedge.com/platform/examples/northwind/northwind.sql \
    | kubectl cnpg psql pgedge-n1 -- -U app app
```

Now, try querying one of the new tables from the another node:

```sh
kubectl cnpg psql pgedge-n2 -- -U app app -c "select * from northwind.shippers"
```

## Uninstalling the Helm Chart

If you wish to uninstall the pgEdge Helm chart, you can perform a `helm uninstall` with the following command:

```shell
helm uninstall pgedge
```

All resources will be removed, with the exception of secrets which were created to store generated client certificates by `cert-manager`. This is a safety mechanism which aligns with cert-manager's default behavior, and ensures that dependent services are not brought down by an accidental update.

If you wish to delete these secrets, you can identify then via `kubectl`:

```shell
kubectl get secrets

NAME                 TYPE                DATA   AGE
admin-client-cert    kubernetes.io/tls   3      3m43s
app-client-cert      kubernetes.io/tls   3      3m43s
client-ca-key-pair   kubernetes.io/tls   3      3m46s
pgedge-client-cert   kubernetes.io/tls   3      3m45s
```

Then, you can delete each secret using the following command:

`kubectl delete secret <name>`
