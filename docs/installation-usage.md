# Installation & Usage Guide

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
        minSyncReplicas: 1
        maxSyncReplicas: 2
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

## Installation & Setup

### 0. Ensure you have the Proper Context Created

If not, the following steps will help:

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

### 1. Configure your context and namespace

For convenience, configure your desired context and namespace prior to running the rest of the commands.

```shell
kubectl config use-context <cluster-context> --namespace <desired-namespace>
```

For example:
```shell
kubectl config use-context helm-test --namespace pgedge
```

### 2. Install Dependencies

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

### 3. Install the Helm Chart

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

#### Command Breakdown

- `helm install`: The main command to deploy a Helm chart.
- `--values examples/configs/single/values.yaml`: This flag tells Helm to use a specific configuration file. The path is relative to your current directory.
- `--wait`: This flag ensures that the command waits until all the resources in the chart are ready before marking the installation as complete.
- `pgedge`: This is the **release name** for your Helm chart. You can name this anything you want; it's a unique identifier for your installation.
- `./`: The dot (`.`) at the end is a relative path that tells Helm to install the chart located in the **current directory**.

**NOTE:** This command may take a long time to run depending on your configuration. You can monitor the progress of the Spock initialization job with this command:

```shell
kubectl logs --follow jobs/pgedge-init-spock
```

## Usage

### Checking Status

You can view the status of a cluster with this command. The section labeled "Unmanaged Replication Slot Status" shows the Spock replication slots:

```shell
kubectl cnpg status pgedge-n1 -v
```

### Viewing Logs

Use this command to view logs for specific nodes. Replace `pgedge-n1` with the name of the node. This will use the `cnpg` plugin to format the logs for easy viewing.

```shell
kubectl cnpg logs cluster pgedge-n1 | kubectl cnpg logs pretty
```

### Connecting to the database

To connect to a specific database node, use the `kubectl cnpg psql` command with the appropriate details for your cluster.

The full command structure is: `kubectl cnpg psql <NODE_NAME> -- -U <USERNAME> <DATABASE_NAME>`

- **`<NODE_NAME>`**: The name of the pgEdge node you want to connect to. In a three-node cluster, these are typically named `pgedge-n1`, `pgedge-n2`, etc.
- **`--`**: This is a separator that tells `kubectl` to pass the following arguments directly to the `psql` command.
- **`-U <USERNAME>`**: The user account you want to connect with.
  - `app`: The default user for application access.
  - `admin`: The superuser with full administrative privileges.
- **`<DATABASE_NAME>`**: The name of the database you want to connect to. The default application database is `app`.

#### Example Commands

**Connect as `app` (standard user)**

To connect to the database named `app` on the node `pgedge-n1` using the `app` user, run:

```shell
kubectl cnpg psql pgedge-n1 -- -U app app
```

**Connect as `admin` (superuser)**

To connect to the database named `admin` on the node `pgedge-n1` using the `admin` superuser, run:

```shell
kubectl cnpg psql pgedge-n1 -- -U admin app
```

### Promoting a replica

If you configured a node with more than one instance you can promote one of the replicas like this:

First, list the instances with `kubectl cnpg status <NODE NAME>`:

```
...
Instances status
Name         Current LSN  Replication role  Status  
----         -----------  ----------------  ------  ........
pgedge-n1-1  0/A003490    Primary           OK      ........
pgedge-n1-2  0/A003490    Standby (sync)    OK      ........
pgedge-n1-3  0/A003490    Standby (sync)    OK      ........
...
```

Then, use this command to promote a replica:

```shell
kubectl cnpg promote pgedge-n1 pgedge-n1-2
```

### Password-based authentication

By default, the managed `app` user is issued a *unique password for each pgEdge node* which is stored in a Kubernetes secret named `pgedge-n#-app`. You can connect to each node using the following approach of fetching the secret and invoking `psql`.

```shell
kubectl run psql-client --rm -it \
  --image=ghcr.io/pgedge/pgedge-postgres:17-spock5-standard \
  --env "PGPASSWORD=$(kubectl get secret pgedge-n3-app -o jsonpath='{.data.password}' | base64 -d)" \
  -- psql -h pgedge-n3-rw -d app -U app
```

### Certificate-based authentication

The pgEdge Helm chart creates certificates for managed users as secrets which you can use in your application for secure authentication and encrypted traffic. Unlike password-based authentication *these are identical across all nodes*. To use them, mount the certificate for the user as a volume in your application's pods like this:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: your-application
spec:
  containers:
  - name: your-application
    image: your-application:latest
    volumeMounts:
    - name: app-client-cert
      mountPath: /certificates/app
      readOnly: true
volumes:
  - name: app-client-cert
    secret:
      secretName: app-client-cert
      items:
        - key: tls.crt
          path: tls.crt
          mode: 0600
        - key: tls.key
          path: tls.key
          mode: 0600
        - key: ca.crt
          path: ca.crt
          mode: 0600
```

Then configure your application to use these certificates when connecting to the Postgres database via a DSN using `sslkey` and `sslcert`.

`host=pgedge-n1-rw dbname=app user=app sslcert=/certificates/app/tls.crt sslkey=/certificates/app/tls.key sslmode=require port=5432`

**NOTE:** The current version of the pgEdge Helm chart does not implement server certificate verification, so the `sslmode` in your DSN should be set to `require`

### Performing backups

CloudNativePG provides multiple ways to configure backups depending on your business requirements. A comparison of the currently available options can be found in their [documentation](https://cloudnative-pg.io/documentation/1.27/backup/#comparing-available-backup-options-object-stores-vs-volume-snapshots).

#### Backups via Barman Cloud CNPG-I plugin

The images utilized in this chart do not contain bundled Barman, and therefore it is required to leverage the Barman Cloud CNPG-I plugin to perform backups / wal archiving.

You can follow these steps to setup backups to an S3 bucket:

1. Install the Barman Cloud CNPG-I plugin

Installation instructions can be accessed in the [Barman Cloud CNPG-I Plugin docs](https://cloudnative-pg.io/plugin-barman-cloud/docs/installation/).

```shell
kubectl apply -f \
        https://github.com/cloudnative-pg/plugin-barman-cloud/releases/download/v0.6.0/manifest.yaml
```

This step assumes you have already installed cert-manager as part of general instructions for this chart. If not, install that according to the documentation.

2. Create an S3 Bucket and issue an Access Key / Secret Access Key for a user which has access to the bucket

3. Create an Kubernetes secret and store your AWS credentials

```sh
kubectl create secret generic aws-creds \
    --from-literal=ACCESS_KEY_ID=<ACCESS_KEY_ID> \
    --from-literal=ACCESS_SECRET_KEY=<ACCESS_SECRET_KEY>
```

4. Create an ObjectStore which points to your S3 bucket and is configured to fetch secrets from the aws-creds secret above:

You can add this template into the `templates` folder or manage it through a separate Helm deployment.

```yaml
apiVersion: barmancloud.cnpg.io/v1
kind: ObjectStore
metadata:
  name: s3-store
spec:
  configuration:
    destinationPath: "s3://<YOUR BUCKET NAME>/path/if/desired"
    s3Credentials:
      accessKeyId:
        name: aws-creds
        key: ACCESS_KEY_ID
      secretAccessKey:
        name: aws-creds
        key: ACCESS_SECRET_KEY
```

Note: You should generally not re-use an ObjectStore across multiple CloudNativePG clusters, but the data will be namespaced with the name of each CloudNativePG cluster (pgedge-n1 for example).

5. Create or update your cluster to configure backups via the plugin

For example, this would enable backups and WAL archiving via the Barman Cloud CNPG-I plugin into the object store defined above:

```yaml

pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec: 
        plugins:
        - name: barman-cloud.cloudnative-pg.io
          isWALArchiver: true
          parameters:
            barmanObjectName: s3-store
    - name: n2
      hostname: pgedge-n2-rw

  clusterSpec:
    storage:
      size: 1Gi
```

6. Once deployed, you can run backups via the `kubectl cnpg` plugin:

```sh
kubectl cnpg backup pgedge-n1 -m plugin --plugin-name barman-cloud.cloudnative-pg.io
```

Once created, you can monitor your backup via kubectl:

```sh
kubectl get backups
```

7. Scheduled backups with Barman can be configured iva the `ScheduledBackup` resource

For example, to setup a scheduled backup at midnight everyday for the `n1` node, use this template:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: scheduled-pgedge-n1
spec:
  schedule: "0 0 0 * * *"
  backupOwnerReference: self
  cluster:
    name: pgedge-n1
  method: plugin
  pluginConfiguration:
    name: barman-cloud.cloudnative-pg.io
```

### Updating your deployment

#### Configuration updates

This chart is built upon passing through configuration details about each node to CloudNativePG. In order to perform configuration updates across your deployed nodes, simply make those updates to the `clusterSpec`, either within an individual node or for all nodes in your deployment.

From there, run a helm upgrade:

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

You can monitor the status of these updates by monitoring each CloudNativePG cluster via `kubectl cnpg status pgedge-n1`.

#### Adding a node via spock

In order to add a node after installing the chart, add a new entry to the `nodes` list with configuration for the new node.

For example, if you have the chart installed with 2 nodes, like in this configuration:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw
  clusterSpec:
    storage:
      size: 1Gi
```

You can add a new node simply by introducing it to the `nodes` list, and use spock to bootstrap it from another node:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw
    - name: n3
      hostname: pgedge-n3-rw
      bootstrap:
        mode: spock
        sourceNode: n1
  clusterSpec:
    storage:
      size: 1Gi
```

From there, perform a helm upgrade to deploy the new node. 

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

The `init-spock` job will run during the upgrade, ensuring that replication configuration is established across the new and existing nodes, and use spock subscription's `sync_data` and `sync_structure` to align the new node via logical replication.

*** NOTE ***

In order to ensure the health of replication across your nodes, you should:

1. Stop writes to all nodes before performing the upgrade
2. Ensure all previous writes have replicated to other nodes by monitoring replication lag via spock

This will ensure that all nodes remain aligned during the update, and replication can continue successfully once you resume writes.

#### Adding a node via CNPG bootstrap

As an alternative approach to adding a node, you can also bootstrap the new node using CloudNativePG's [Bootstrap from another cluster](https://cloudnative-pg.io/documentation/1.27/bootstrap/#bootstrap-from-another-cluster) capability

Here is an example of adding a node `n3` using the Barman Cloud CNPG-I plugin to bootstrap the node from the existing node `n1` which has backups and wal archiving configured in S3. 

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec: 
        plugins:
        - name: barman-cloud.cloudnative-pg.io
          isWALArchiver: true
          parameters:
            barmanObjectName: s3-store
    - name: n2
      hostname: pgedge-n2-rw
    - name: n3
      hostname: pgedge-n3-rw
      bootstrap:
        mode: cnpg
      clusterSpec: 
        bootstrap:
          initdb: null
          recovery:
            source: pgedge-n1
        externalClusters:
        - name: pgedge-n1
          plugin:
            name: barman-cloud.cloudnative-pg.io
            parameters:
              barmanObjectName: s3-store
              serverName: pgedge-n1

  clusterSpec:
    storage:
      size: 1Gi
```

This builds upon the example above in [Performing Backups](#performing-backups).

The init-spock job will reconfigure the restored node, ensuring to maintain existing replication configuration. Regardless of the CloudNativePG bootstrap approach you take, you should ensure that the data being restored on the new node aligns with the state of the other nodes.

#### Removing a node

In order to remove a node from your cluster, remove it from `nodes` in your values.yaml file, and perform a `helm upgrade`:

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

The `init-spock` job will run during the upgrade, ensuring that configuration which references the removed node are cleaned up on the remaining nodes.

## Using `kind` to test locally

For local testing, you can use [kind](https://kind.sigs.k8s.io) to create a test cluster that runs on your machine. You'll need access to a Docker host, such as Docker Desktop.

Use this command to install `kind` with Homebrew:

```shell
brew install kind
```

To deploy a local Kubernetes cluster, install kind and run this command:

```shell
kind create cluster --config examples/configs/single/kind.yaml
```

Next, set your kubectl config to use the kind cluster:

```shell
kubectl config use-context kind-single --namespace default
```

To tear down the `kind` cluster, run this command:

```shell
kind delete cluster --name single
```
