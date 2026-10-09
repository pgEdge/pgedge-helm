# Adding Nodes

This chart supports two methods for adding new nodes to an existing installation:

1. Using Spock to add a node via logical replication
2. Using CloudNativePG to add a node via [bootstrap](https://cloudnative-pg.io/docs/1.29/bootstrap)

## Adding a node via spock

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

You can add a new node simply by introducing it to the `nodes` list, and including the `mode: spock` property to bootstrap it from another node:

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

The `init-spock` job will run during the upgrade, ensuring that replication configuration is established across the new and existing nodes, and use Spock subscription's `sync_data` and `sync_structure` to align the new node via logical replication. Writes can continue uninterrupted on existing nodes throughout this process.

!!! note

    For large databases, the initial sync may take significant time. You can configure the timeout via `pgEdge.initSpockJobConfig.timeout` (default: 7200 seconds / 2 hours). If the job fails or times out, see [Recovering from a failed add](#recovering-from-a-failed-add).

### Adding a node running a different Spock version

A new node must run the same Spock major version as every existing node.
Upstream Spock has a fallback that in principle lets a newer add-node sync
worker read an older peer's pre-6.0 progress table directly, but as of the
current `ghcr.io/pgedge/pgedge-postgres:18-spock6-standard` preview image,
that fallback isn't present: adding a Spock 6 node to a Spock 5 cluster
crash-loops the new node's apply worker (`function
spock.read_peer_progress(...) does not exist`, called directly against the
Spock 5 peer with no fallback). The `init-spock` job checks the Spock major
version of every node before making any changes and fails with a clear error
on any mismatch, in either direction. This is a limitation of the currently
published preview image, not a permanent restriction — it may be lifted in a
future preview build.

This preview does not provide a supported Spock 5-to-6 upgrade procedure for
an existing cluster. The validation above only covers add-node; it does not
verify that changing an existing node's image in place, one node at a time,
keeps replication working across the version gap in between — that path is
untested and not recommended.

!!! warning

    Remove the `bootstrap` block from the new node's configuration after a successful add. If left in place, subsequent `helm upgrade` runs will re-execute the populate pipeline, which may interfere with active replication.

## Adding a node via CloudNativePG bootstrap

As an alternative approach to adding a node, you can also bootstrap the new node using CloudNativePG's [Bootstrap from another cluster](https://cloudnative-pg.io/docs/1.29/bootstrap/#bootstrap-from-another-cluster) capability.

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

This builds upon the example configuration started in [Performing Backups](backups.md).

The subsequent init-spock job reconfigures the restored node, ensuring replication remains healthy after performing the update.

Regardless of the CloudNativePG bootstrap approach you take, you should ensure that the data being restored on the new node aligns with the state of the other nodes.

## Recovering from a failed add

If the init-spock job fails while adding a node (e.g. due to a crash, timeout, or connectivity issue), the new node may be left in a partially configured state. To retry, first remove the node from the `nodes` list in `values.yaml` and run `helm upgrade` to clean up, then re-add the node and upgrade again.

Do not retry the add without first removing the node, as the reconciler may encounter stale replication state from the failed attempt.
