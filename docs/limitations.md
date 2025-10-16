# Limitations

The following limitations apply when deploying `pgedge-helm`.

## Certificate Management

The chart creates a self-signed CA and issues certificates for managed users on each node, but it does not issue or configure server certificates.

You may manage server certificates yourself using cert-manager and CloudNativePG, but the chart may require modification to use verify-full mode for client connections.

## Node Management

Adding nodes is supported by updating the `pgEdge.nodes` value and running a helm upgrade, but writes must be stopped on existing nodes during the upgrade.

See [Adding Nodes](usage/adding_nodes.md) for more information.

## App database name

The app database is named "app" by default, and cannot be renamed without modifying various Helm templates and the default values.yaml file.

## Installing multiple times into the same namespace

Installing the chart multiple times into the same Kubernetes namespace is not recommended. Each release expects to manage its own set of resources, and resource name conflicts or unexpected behavior may occur if multiple releases are installed in the same namespace.

For testing purposes, you can modify the `appName` property to separate most chart resources across multiple helm deployments in the same namespace. However, certificates deployed when `provisionCerts` is set to `true` will have the same name across all deployments, and these will conflict in the same namespace.