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
- [kubectl cnpg plugin](https://cloudnative-pg.io/documentation/current/kubectl-plugin/#install): A plugin for `kubectl` that provides additional commands for managing CloudNativePG clusters.

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

First, install the `CloudNativePG` and `cert-manager` operators into your cluster:

```shell
kubectl apply --server-side -f \
https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.28/releases/cnpg-1.28.0.yaml

kubectl apply -f \
https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml

kubectl wait --for=condition=Available deployment \
	-n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=120s
```

## Step 3: Install the chart

!!! tip "Restricted Namespaces"
    pgEdge Helm is compatible with Kubernetes namespaces using the `restricted` Pod Security Standard. See the [Security](security.md) guide for details on security contexts and hardened deployments.

Download the latest `pgedge-helm` release package from [pgEdge Helm Releases](https://github.com/pgEdge/pgedge-helm/releases/).  

After downloading and extracting the package, run the `helm install` command from the directory containing the extracted `pgedge-helm` chart.

1. **Navigate to the Correct Directory**  
   First, change your current directory to the location where you unzipped/downloaded the Helm chart.

2. **Run the Helm Install Command**  
   Once you are in the `pgedge-helm` directory, you can run the `helm install` command. The command uses relative paths, which is why changing directories first is crucial.

```shell
helm install \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

### Command Breakdown

- `helm install`: The main command to deploy a Helm chart.
- `--values examples/configs/single/values.yaml`: This flag tells Helm to use a specific configuration file. The path is relative to your current directory.
- `--wait`: This flag ensures that the command waits until all the resources in the chart are ready before marking the installation as complete.
- `pgedge`: This is the **release name** for your Helm chart. You can name this anything you want; it's a unique identifier for your installation.
- `./`: The dot (`.`) at the end is a relative path that tells Helm to install the chart located in the **current directory**.

**NOTE:** This command may take a long time to run depending on your configuration. You can monitor the progress of the Spock initialization job with this command:

```shell
kubectl logs --follow jobs/pgedge-init-spock
```

Once the job has completed, you should see the following message which indicates the chart has been successfully installed.

```shell
➜ helm install \
--values examples/configs/single/values.yaml \
        --wait \
        pgedge ./

NAME: pgedge
LAST DEPLOYED: Fri Oct 10 14:07:41 2025
NAMESPACE: pgedge
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

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
