## Minor Version Upgrades

This chart utilizes a mutable tag by default to pull the latest pgEdge Enterprise Postgres image for Postgres 17 and Spock 5.

The use of this mutable tag in the chart makes it easier to keep examples updated as new versions of Postgres and Spock are released.

```yaml
clusterSpec:
  imagePullPolicy: Always
  imageName: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
```

!!! note

    When deploying this chart in your production environment, we recommend pinning to the latest immutable tag for your desired versions and image flavor.

You can find a tag for your desired Postgres and Spock version in the [pgEdge GitHub Container Registry](https://github.com/pgEdge/postgres-images/pkgs/container/pgedge-postgres).

For example, this pins to a specific Postgres minor version and Spock minor version for the `standard` image flavor.

```yaml
clusterSpec:
  imagePullPolicy: Always
  imageName: ghcr.io/pgedge/pgedge-postgres:17.6-spock5.0.1-standard
```

In order to perform minor version upgrades for Postgres or Spock, simply update the `imageName` and perform a `helm upgrade`. CloudNativePG will handle rolling out the new image across your nodes.

## Major Version Upgrades

For Postgres major version upgrades, this chart supports two strategies:

- Using Spock to bootstrap new nodes with a new major version.
- Performing an in-place upgrade via CloudNativePG to a new major version.

Both approaches require stopping all writes from your applications to existing nodes in order to ensure data remains aligned during the upgrade process.

In addition to stopping writes from your applications, you should also ensure that outstanding writes have been replicated to all existing nodes before performing an upgrade.

!!! info

    Postgres 18 introduces stricter pg_upgrade verification: any negative or high WAL lag in logical replication slots can cause the upgrade to fail. You can monitor existing replication lag for  using the `spock.lag_tracker` view on each node. The `replication_lag_bytes` column should be zero across all subscriptions before proceeding.

!!! warning

    You should thoroughly test upgrade scenarios in a separate environment before proceeding on your production database. In addition, you should take backups of the existing data using CloudNativePG, and be prepared to restore in the event of unforseen problems.

### Upgrading major versions via Spock bootstrap

If your database schema is compatible across major versions, you can use Spock to bootstrap upgraded nodes into your installation in order to gradually perform a major version upgrade.

For example, let's assume you have 2 nodes running Postgres 16:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw

  clusterSpec:
    imageName: ghcr.io/pgedge/pgedge-postgres:16-spock5-standard
    storage:
      size: 1Gi
```

You can add a new node (n3) running Postgres 17, with bootstrap configuration to load data from an existing node running Postgres 16:

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
        imageName: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard

  clusterSpec:
    imageName: ghcr.io/pgedge/pgedge-postgres:16-spock5-standard
    storage:
      size: 1Gi
```

Using this approach, you can gradually upgrade to Postgres 17 by removing existing Postgres 16 nodes.

### Performing in-place upgrades via CloudNativePG

CloudNativePG supports in-place major version upgrades using its [Offline In-Place Major Upgrades](https://cloudnative-pg.io/documentation/1.27/postgres_upgrades/#offline-in-place-major-upgrades) approach.

In order to perform a major version upgrade using this chart, simply update the `imageName` in the `clusterSpec` and perform a `helm upgrade`.

For example, given the following two node configuration running Postgres 16:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw
  clusterSpec:
    imageName: ghcr.io/pgedge/pgedge-postgres:16-spock5-standard
    storage:
      size: 1Gi
```

You can perform a major version upgrade from 16 to 17 by specifying a new image for Postgres 17:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
    - name: n2
      hostname: pgedge-n2-rw

  clusterSpec:
    imageName: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
    storage:
      size: 1Gi
```

After running `helm upgrade`, CloudNativePG will orchestrate the upgrade, performing checks and replacing pods as needed.

You can monitor upgrade progress using `kubectl get clusters` or `kubectl cnpg status <node>`.

For detailed instructions and troubleshooting for in-place upgrades, refer to the [CloudNativePG documentation](https://cloudnative-pg.io/documentation/1.27/postgres_upgrades/#offline-in-place-major-upgrades).
