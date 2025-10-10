CloudNativePG provides multiple ways to configure backups depending on your business requirements. A comparison of the currently available options can be found in their [documentation](https://cloudnative-pg.io/documentation/1.27/backup/#comparing-available-backup-options-object-stores-vs-volume-snapshots).

## Backups via Barman Cloud CNPG-I plugin

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