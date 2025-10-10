This chart is built upon passing through configuration details about each node to CloudNativePG. In order to perform configuration updates across your deployed nodes, simply make those updates to the `clusterSpec`, either within an individual node or for all nodes in your deployment.

From there, run a helm upgrade:

```shell
helm upgrade \
--values examples/configs/single/values.yaml \
	--wait \
	pgedge ./
```

You can monitor the status of these updates by monitoring each CloudNativePG cluster via `kubectl cnpg status pgedge-n1`.