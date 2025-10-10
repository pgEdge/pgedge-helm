## Minor Version Upgrades

This chart utilizes a mutable tag by default to pull the latest spock 5 image for Postgres 17. The use of this mutable tag in the chart makes it easier to keep examples updated as new versions of Postgres and spock are released.

```yaml
  clusterSpec:
    imagePullPolicy: Always
    imageName: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
```

When deploying this chart in your production environment, we recommend pinning to the latest immutable tag for your desired versions and image flavor:

You can find a tag for your desired Postgres and spock version in the [pgEdge GitHub Container Registry](https://github.com/pgEdge/postgres-images/pkgs/container/pgedge-postgres).

For example, this pins to a specific Postgres minor version and spock minor version for the `standard` image flavor.

```yaml
  clusterSpec:
    imagePullPolicy: Always
    imageName: ghcr.io/pgedge/pgedge-postgres:17.6-spock5.0.1-standard
```

In order to perform minor version upgrades for Postgres or spock, simply update the imageName and perform a `helm upgrade`. CloudNativePG will handle rolling out the new image across your nodes.

## Major Version Upgrades

For major version upgrades, this chart does not currently support performing in-place major upgrades for existing nodes. 

If your database is compatible, you can use spock to bootstrap upgraded nodes into your cluster in order to gradually perform a major version upgrade. 

This operation requires stopping all writes to existing nodes in order to ensure data remains aligned during the upgrade process.

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

You can add a new node (n3) running Postgres 17, with bootstrap configuration to load data from n1:

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

From there, you could remove existing Postgres 16 nodes to gradually eliminate them from your cluster.

**NOTE:** You should thoroughly test upgrade scenarios in a separate environment before proceeding on your production database.