# Configuring Backups

CloudNativePG provides multiple ways to configure backups depending on your business requirements. A comparison of the currently available options can be found in their [documentation](https://cloudnative-pg.io/documentation/1.28/backup/#comparing-available-backup-options-object-stores-vs-volume-snapshots).

This chart supports deploying backup-related resources like `ObjectStore` and `ScheduledBackup` using the `extraResources` field. This allows you to manage all your pgEdge infrastructure in a single Helm deployment.

!!! warning

    The images utilized in this chart do not contain Barman, and therefore it is required to leverage the Barman Cloud CNPG-I plugin to perform backups / wal archiving.

## Backups via Barman Cloud CNPG-I plugin

You can follow these steps to setup scheduled backups to an AWS S3 bucket using the Barman Cloud CNPG-I plugin.

!!! note
    The Barman Cloud CNPG-I plugin supports additional ObjectStore providers, including Microsoft Azure Blob Storage, Google Cloud Storage, and additional S3-compatible services such as MinIO, Linode Object Storage, and Digital Ocean Spaces. 

    For more information, refer to the [Barman Cloud CNPG-I plugin documentation](https://cloudnative-pg.io/plugin-barman-cloud/docs/object_stores/).

1.  Install the Barman Cloud CNPG-I plugin

    The plugin is available from the pgEdge Helm repository:

    ```shell
    helm repo add pgedge https://pgedge.github.io/charts
    helm repo update
    helm install plugin-barman-cloud pgedge/plugin-barman-cloud \
      --namespace cnpg-system
    ```

    This step assumes you have already installed `cert-manager` as part of general instructions for this chart. If not, install that according to the [cert-manager documentation](https://cert-manager.io/docs/installation/).

2.  Create an S3 Bucket and issue an Access Key / Secret Access Key for a user which has access to the bucket.

3.  Create a Kubernetes secret and store your AWS credentials.

    ```sh
    kubectl create secret generic aws-creds \
        --from-literal=ACCESS_KEY_ID=<ACCESS_KEY_ID> \
        --from-literal=ACCESS_SECRET_KEY=<ACCESS_SECRET_KEY>
    ```

4.  Configure backups via the plugin with `ObjectStore` resource.

    Add the `ObjectStore` resource using the `extraResources` field in your Helm values. The following configuration creates an ObjectStore pointing to your S3 bucket and configures the cluster to use it for backups and WAL archiving:

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

      extraResources:
        - apiVersion: barmancloud.cnpg.io/v1
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

    !!! note

        You should generally not re-use an ObjectStore across multiple CloudNativePG clusters, but the data will be namespaced with the name of each CloudNativePG cluster (pgedge-n1 for example).

5.  Deploy or upgrade your Helm release with the updated configuration.

    ```shell
    helm upgrade --install pgedge ./ --values your-values.yaml --wait
    ```

6.  Once deployed, run backups via the `kubectl cnpg` plugin.

    ```sh
    kubectl cnpg backup pgedge-n1 -m plugin --plugin-name barman-cloud.cloudnative-pg.io
    ```

    Once created, you can monitor your backup via kubectl:

    ```sh
    kubectl get backups
    ```

7.  Configure scheduled backups (optional).

    If desired, configure scheduled backups by adding a `ScheduledBackup` resource to `extraResources`. For example, to setup a scheduled backup at midnight every day for the `n1` node:

    ```yaml
    pgEdge:
      appName: pgedge
      extraResources:
        - apiVersion: barmancloud.cnpg.io/v1
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
        - apiVersion: postgresql.cnpg.io/v1
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
