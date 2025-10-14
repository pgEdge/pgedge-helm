This chart supports two methods for adding new nodes to an existing installation:

1. Using spock to add a node via logical replication
2. Using CloudNativePG to add a node via [bootstrap](https://cloudnative-pg.io/documentation/1.18/bootstrap/)

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

!!! note

    In order to ensure the health of replication across your nodes, you should:

    1. Stop writes to all nodes before performing the upgrade
    2. Ensure all previous writes have replicated to other nodes by monitoring replication lag via spock

    This will ensure that all nodes remain aligned during the update, and replication can continue successfully once you resume writes.

## Adding a node via CloudNativePG bootstrap

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

This builds upon the example establish in [Performing Backups](backups.md).

The init-spock job will reconfigure the restored node, ensuring to maintain existing replication configuration. 

Regardless of the CloudNativePG bootstrap approach you take, you should ensure that the data being restored on the new node aligns with the state of the other nodes.
