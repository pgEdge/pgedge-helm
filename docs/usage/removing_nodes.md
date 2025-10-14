In order to remove a node from your installation, remove it from `nodes` in your values.yaml file, and perform a `helm upgrade`:

For example, if your existing installation has three nodes:

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
  clusterSpec:
    storage:
      size: 1Gi
```

You can update your installation to remove n3 from the `nodes` list:

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

Next, perform a `helm upgrade` to apply the new configuration:

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
    --wait \
    pgedge ./
```

The `init-spock` job will run during the upgrade, ensuring that configuration which references the removed node are cleaned up on the nodes that remain.
