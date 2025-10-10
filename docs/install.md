# Installation

This guide uses the single cluster example to install the pgEdge Helm chart into a single Kubernetes cluster with three nodes (n1, n2, n3).

n1 is configured with 3 instances (1 primary, 2 standby), and n2/n3 are configured with just 1 primary.

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

In order to run through all steps, you'll need the following tools installed on your machine:

- `helm` – [https://helm.sh/](https://helm.sh/)
  - Homebrew install command: `brew install helm`
- `kubectl` – [https://kubernetes.io/docs/tasks/tools/#kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
  - Homebrew install command: `brew install kubectl`
- `kubectl` CloudNativePG plugin – [https://cloudnative-pg.io/documentation/current/kubectl-plugin/](https://cloudnative-pg.io/documentation/current/kubectl-plugin/)
  - Homebrew install command: `brew install kubectl-cnpg`

## Creating your kubectl context

Before you get started, you should setup your `kubectl` context to interact with the correct cluster and namespace before performing the install.

**Identify Your Cluster and User**

First, you need to know the names of the cluster and user you want to use. You can list them with these commands:

```shell
kubectl config get-clusters
kubectl config get-users
```

Let's assume your cluster is named `kubernetes` and your user is named `kubernetes-admin`.

**Create the New Context**

Now, create the new context using the cluster and user names you just found.

```shell
kubectl config set-context helm-test --cluster=kubernetes --user=kubernetes-admin
```

This command creates a new context and links it to your existing cluster and user credentials.

## Configuring your kubectl context and namespace

For convenience, configure your desired context and namespace prior to running the rest of the commands.

```shell
kubectl config use-context <cluster-context> --namespace <desired-namespace>
```

For example:

```shell
kubectl config use-context helm-test --namespace pgedge
```

## Installing chart dependencies

First, install the `CloudNativePG` and `cert-manager` operators into your cluster:

```shell
kubectl apply --server-side -f \
https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.27/releases/cnpg-1.27.0.yaml
```

```shell
kubectl apply -f \
https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

```shell
kubectl wait --for=condition=Available deployment \
	-n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=120s
```

## Install the chart

To install the Helm chart, you need to run the `helm install` command from the correct directory. This command needs access to two key parts of the downloaded `pgedge-helm` package: the chart itself (the `./` at the end) and the configuration file (`values.yaml`).

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

## Spock initialization

This chart contains a python job to initialize spock multi-master replication across all nodes once they are all available.

This job runs by default, waiting for any clusters associated with the current deployment to be ready before performing initialization.

If you wish to disable this behavior, you can set `pgEdge.initSpock` to `false`.

### snowflake.node and lolor.node

This chart automatically configures `snowflake.node` and `lolor.node` based on the `name` property of each node.

For example, a node named `n1` will have the following Postgres configuration applied to the node to ensure snowflake and lolor are configured appropriately:

```yaml
  postgresql:
    parameters:
      ...
      lolor.node: "1"
      snowflake.node: "1"
```

If you wish to override this behavior, or plan to utilize alternate naming schemes for your node, you can set the `ordinal` property for each node:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: a
      hostname: pgedge-a-rw
      ordinal: 1
    - name: b
      hostname: pgedge-b-rw
      ordinal: 2
    - name: c
      hostname: pgedge-c-rw
      ordinal: 3
  clusterSpec:
    storage:
      size: 1Gi
```