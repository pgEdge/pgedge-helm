# Installation

pgEdge Helm supports both Enterprise and Distributed deployment models. This grants you flexibility to deploy across one or more regions with the same chart.

This guide demonstrates how to install pgEdge Helm into a single Kubernetes cluster with the distributed deployment model.

This example uses three pgEdge nodes: n1, n2, and n3 deployed into a single Kubernetes cluster.

n1 is configured with 3 instances (1 primary, 2 standby), and n2/n3 are configured with just 1 primary instance.

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

You can also follow this guide for single region deployments with pgEdge Enterprise Postgres. 

To do this, specify a single node `n1` under `nodes` with standby instances configured:

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

If you want to extend this configuration later to use a distributed architecture, see [Adding Nodes](usage/adding_nodes.md).

**Prerequisites**

To perform the installation, you'll need the following tools installed on your machine:

- [helm](https://helm.sh/docs/intro/install/): The package manager for Kubernetes, used to install, upgrade, and manage Kubernetes applications via Helm charts.
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl): The Kubernetes command-line tool, used to interact with and manage your Kubernetes clusters.
- [kubectl cnpg plugin](https://github.com/pgEdge/pgedge-cnpg-dist/releases?q=kubectl-cnpg&expanded=true): A plugin for `kubectl` that provides additional commands for managing CloudNativePG clusters. Download the appropriate binary for your platform from the pgEdge distribution releases.

## Step 1: Configure your kubectl context and namespace

Before you get started, you should setup your `kubectl` context to interact with the correct cluster and namespace before performing the install.

**Identify Your Kubernetes Cluster and User**

First, you need to know the names of the cluster and user you want to use. You can list them with these commands:

```shell
kubectl config get-clusters
kubectl config get-users
```

The example that follows uses a cluster named `kubernetes`, accessed by a user named `kubernetes-admin`.

**Create the New Context**

Now, create the new context using the cluster and user names you just found.

```shell
kubectl config set-context helm-test --cluster=kubernetes --user=kubernetes-admin
```

This command creates a new context and links it to your existing cluster and user credentials.

**Configuring your kubectl context and namespace**

For convenience, configure your desired context and namespace prior to running the rest of the commands.

```shell
kubectl config use-context <cluster-context> --namespace <desired-namespace>
```

For example:

```shell
kubectl config use-context helm-test --namespace pgedge
```

## Step 2: Install chart dependencies

First, add the pgEdge Helm repository and install the `CloudNativePG` and `cert-manager` operators:

```shell
# Add the pgEdge Helm repository
helm repo add pgedge https://pgedge.github.io/charts
helm repo update

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

## Step 3: Install the chart

!!! tip "Restricted Namespaces"
    pgEdge Helm is compatible with Kubernetes namespaces using the `restricted` Pod Security Standard. See the [Security](security.md) guide for details on security contexts and hardened deployments.

### Install from pgEdge Helm Repository

The recommended method is to install directly from the pgEdge Helm repository:

```shell
helm install pgedge pgedge/pgedge \
  --values values.yaml \
  --wait
```

You'll need to create a `values.yaml` file with your configuration. See the example configurations:

- Single region: [examples/configs/single/values.yaml](https://github.com/pgEdge/pgedge-helm/blob/main/examples/configs/single/values.yaml)

### Install from Local Chart

Alternatively, you can download and install from a local chart package:

1. Download the latest release from [pgEdge Helm Releases](https://github.com/pgEdge/pgedge-helm/releases/)
2. Extract the package and navigate to the chart directory
3. Install using the local chart:

```shell
helm install pgedge ./ \
  --values examples/configs/single/values.yaml \
  --wait
```

### Command Details

- `helm install`: Deploys a Helm chart
- `pgedge`: The release name for your installation (can be customized)
- `pgedge/pgedge` or `./`: The chart to install (from repository or local path)
- `--values`: Path to your configuration file
- `--wait`: Waits for all resources to be ready before completing

**NOTE:** This command may take a long time to run depending on your configuration. You can monitor the progress of the Spock initialization job with this command:

```shell
kubectl logs --follow jobs/pgedge-init-spock
```

Once the installation completes, you should see output similar to:

```shell
NAME: pgedge
LAST DEPLOYED: Fri Oct 10 14:07:41 2025
NAMESPACE: pgedge
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

This confirms the chart has been successfully installed.

## Uninstallation

If you wish to uninstall the pgEdge Helm chart, you can perform a `helm uninstall` using the following command:

```shell
helm uninstall pgedge
```

All resources will be removed, with the exception of secrets which were created to store generated client certificates by `cert-manager`.

This is a safety mechanism which aligns with cert-manager's default behavior, and ensures that dependent services are not brought down by an accidental update.

If you wish to delete these secrets, you can identify then via `kubectl`:

```shell
kubectl get secrets

NAME                 TYPE                DATA   AGE
admin-client-cert    kubernetes.io/tls   3      3m43s
app-client-cert      kubernetes.io/tls   3      3m43s
client-ca-key-pair   kubernetes.io/tls   3      3m46s
pgedge-client-cert   kubernetes.io/tls   3      3m45s
```

From there, you can delete each secret using the following command:

`kubectl delete secret <name>`
