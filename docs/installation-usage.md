# Installation & Usage Guide

This guide uses the single cluster example to install the pgEdge Helm chart into a single Kubernetes cluster with three nodes (n1, n2, n3). 

n1 is configured with 3 instances (1 primary, 2 standby), and n2/n3 are configured with just 1 primary.

```
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

In order to run through all steps, you’ll need the following tools installed on your machine:

- `helm` – [https://helm.sh/](https://helm.sh/)  
  - Homebrew install command: `brew install helm`
- `kubectl` – [https://kubernetes.io/docs/tasks/tools/#kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)  
  - Homebrew install command: `brew install kubectl`
- `kubectl` CNPG plugin – [https://cloudnative-pg.io/documentation/current/kubectl-plugin/](https://cloudnative-pg.io/documentation/current/kubectl-plugin/)  
  - Homebrew install command: `brew install kubectl-cnpg`

In addition, you'll need a Kubernetes cluster in which to install the chart. If you wish to setup a local Kubernetes cluster, you can use `kind`. See [Using kind to test locally](#using-kind-to-test-locally).

### 1\. Configure your context and namespace

For convenience, configure your desired context and namespace prior to running the rest of the commands.

```shell
kubectl config use-context <cluster-context> --namespace <desired-namespace>
```

### 2\. Install Dependencies

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

### 3\. Install the Helm chart

Next you’ll deploy the Helm chart into your `kind` cluster:

```shell
helm install \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

**NOTE:** This command may take a long time to run depending on your configuration. You can monitor the progress of the Spock initialization job with this command:

```shell
kubectl logs --follow jobs/pgedge-init-spock
```

## Usage

### Checking Status

You can view the status of a cluster with this command. The section labeled “Unmanaged Replication Slot Status” shows the Spock replication slots:

```shell
kubectl cnpg status pgedge-n1 -v
```

### Viewing Logs

Use this command to view logs for specific nodes. Replace `pgedge-n1` with the name of the node. This will use the `cnpg` plugin to format the logs for easy viewing.

```shell
kubectl cnpg logs cluster pgedge-n1 | kubectl cnpg logs pretty
```

### Connecting to the database

You can connect to the primary instance for each pgEdge node individually using this commands:

```shell
kubectl cnpg psql pgedge-n1 -- -U <USERNAME> app
```

Replace `pgedge-n1` with the name of the node to connect to, and `<USERNAME>` with the username to connect as. The defaults are:

- `app`  
- `admin` (superuser)

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

```
kubectl run psql-client --rm -it \
  --image=ghcr.io/pgedge/pgedge-postgres:17-spock5-standard \
  --env "PGPASSWORD=$(kubectl get secret pgedge-n3-app -o jsonpath='{.data.password}' | base64 -d)" \
  -- psql -h pgedge-n3-rw -d app -U app
```

### Certificate-based authentication

The pgEdge Helm chart creates certificates for managed users as secrets which you can use in your application for secure authentication and encrypted traffic. Unlike password-based authentication *these are identical across all nodes*. To use them, mount the certificate for the user as a volume in your application’s pods like this:

```
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

## Using `kind` to test locally

For local testing, you can use [kind](https://kind.sigs.k8s.io) to create a test cluster that runs on your machine. You’ll need access to a Docker host, such as Docker Desktop.

Use this command to install `kind` with Homebrew:

```
brew install kind
```

To deploy a local Kubernetes cluster, install kind and run this command:

```shell
kind create cluster --config examples/configs/single/kind.yaml
```

Next, set your kubectl config to use the 

```shell
kubectl config use-context kind-single --namespace default
```

To tear down the `kind` cluster, run this command:

```shell
kind delete cluster --name single
```