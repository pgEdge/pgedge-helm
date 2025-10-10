In order to remove a node from your cluster, remove it from `nodes` in your values.yaml file, and perform a `helm upgrade`:

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
    --wait \
    pgedge ./
```

The `init-spock` job will run during the upgrade, ensuring that configuration which references the removed node are cleaned up on the remaining nodes.