# Configuring Backups

CloudNativePG provides multiple ways to configure backups depending on your business requirements. A comparison of the currently available options can be found in their [documentation](https://cloudnative-pg.io/docs/1.29/backup/#comparing-available-backup-options-object-stores-vs-volume-snapshots).

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


## Verify a backup with a restore

Use CloudNativePG's native [recovery procedure](https://cloudnative-pg.io/docs/1.29/recovery/)
to check a single node's physical backup. The steps below use `pgedge-n1` from
the configuration above and require only `kubectl`, including the `psql` client
already in the PostgreSQL container.

!!! warning

    Use an isolated test environment that cannot connect to live pgEdge nodes.
    The restored database also contains Spock metadata. This check does not
    rebuild the multi-active topology, verify replication, or rejoin a failed
    node. Do not add this disposable Cluster to `pgEdge.nodes` or run the
    chart's Spock initialization job against it.

1.  Choose a completed plugin backup for `pgedge-n1` and record its **backup ID**,
    which is different from the Kubernetes Backup resource name:

    ```sh
    kubectl get backups -o custom-columns=NAME:.metadata.name,CLUSTER:.spec.cluster.name,METHOD:.spec.method,PHASE:.status.phase,BACKUP_ID:.status.backupId
    kubectl get cluster pgedge-n1 -o yaml
    ```

    Commands assume the namespace of the pgEdge release; add `-n <namespace>`
    if needed. Copy the source image, PostgreSQL configuration, storage class
    and capacity, and any separate WAL-storage settings. Keep the source's
    `snowflake.node` and `lolor.node` values in this isolated copy.

2.  Create a separate recovery `ObjectStore` named `pgedge-restore-store` in
    that namespace, using the [Barman plugin configuration](https://cloudnative-pg.io/plugin-barman-cloud/docs/usage/#restoring-a-cluster).
    Point it at the same archive, but use credentials limited to reading the
    required base backups and WAL. Match the endpoint, compression, encryption,
    and any required noncredential S3 compatibility settings.

3.  Save the following as `pgedge-restore-check.yaml`. Replace `<SOURCE_IMAGE>`,
    `<SOURCE_STORAGE_SIZE>` and `<BACKUP_ID>` with the values recorded above.
    The PostgreSQL settings shown are for this chart's default `n1` configuration;
    adjust them to match the source, including its shared libraries. Set a
    suitable storage class and separate WAL storage if the source uses them.

    ```yaml
    apiVersion: postgresql.cnpg.io/v1
    kind: Cluster
    metadata:
      name: pgedge-restore-check
    spec:
      instances: 1
      imageName: "<SOURCE_IMAGE>"
      storage:
        size: "<SOURCE_STORAGE_SIZE>"
      postgresql:
        parameters:
          snowflake.node: "1"
          lolor.node: "1"
          track_commit_timestamp: "on"
          wal_level: logical
        shared_preload_libraries:
          - pg_stat_statements
          - snowflake
          - spock
      bootstrap:
        recovery:
          source: pgedge-n1
          recoveryTarget:
            backupID: "<BACKUP_ID>"
            targetImmediate: true
      externalClusters:
        - name: pgedge-n1
          plugin:
            name: barman-cloud.cloudnative-pg.io
            parameters:
              barmanObjectName: pgedge-restore-store
              serverName: pgedge-n1
    ```

    `serverName` is the source's archive server name, normally `pgedge-n1`.
    Use the source plugin's override if it has one, not the disposable Cluster's
    name. `backupID` pins the chosen base backup; `targetImmediate` stops recovery
    when that backup first reaches consistency. No WAL archiver is configured
    for the restored Cluster.

4.  Create the restore and watch it become ready:

    ```sh
    kubectl apply --dry-run=server -f pgedge-restore-check.yaml
    kubectl apply -f pgedge-restore-check.yaml
    kubectl get pods -l cnpg.io/cluster=pgedge-restore-check -w
    ```

    Stop watching with Ctrl-C, then check readiness. Increase the timeout for
    larger databases; inspect the Cluster events and recovery Pod logs if it fails.

    ```sh
    kubectl wait --for=condition=Ready cluster/pgedge-restore-check --timeout=10m
    kubectl describe cluster pgedge-restore-check
    ```

5.  Read the restored database, using the primary Pod name reported by the Cluster:

    ```sh
    RESTORE_POD=$(kubectl get cluster pgedge-restore-check -o jsonpath='{.status.currentPrimary}')
    kubectl exec "$RESTORE_POD" -c postgres -- psql -X -v ON_ERROR_STOP=1 -d app -c \
      "BEGIN READ ONLY; SELECT pg_is_in_recovery(), EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'spock') AS spock_present; COMMIT;"
    ```

    Expect `pg_is_in_recovery` to be `f` and `spock_present` to be `t`. Also run
    read-only assertions against known application records from before the backup.
    A ready Pod and an installed extension alone do not verify application data.
    Keep the chosen backup ID, SQL results and recovery duration with your test record.

6.  Remove only the disposable Cluster and its PVCs after reviewing the results:

    ```sh
    kubectl delete cluster pgedge-restore-check --wait=true
    kubectl delete pvc -l cnpg.io/cluster=pgedge-restore-check
    kubectl get pods,pvc -l cnpg.io/cluster=pgedge-restore-check
    ```

    Check that no matching resources remain. Storage reclaim policies determine
    whether deleting a PVC also removes its backing volume; check retained volumes
    separately. Keep the source Cluster, its backups and archive unchanged.
