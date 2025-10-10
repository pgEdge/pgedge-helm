## Certificate Management

The chart creates a self-signed CA and issues certificates for managed users on each node, but it does not issue or configure server certificates.

You may manage server certificates yourself using cert-manager and CloudNativePG, but the chart may require modification to use verify-full mode for client connections.

## Node Management

Adding nodes is supported by updating the `pgEdge.nodes` value and running a helm upgrade, but writes must be stopped on existing nodes during the upgrade.

See the [Adding nodes](usage/adding_nodes.md) for more information.

## App database name

The app database is named "app" by default, and cannot be renamed without modifying various Helm templates and the default values.yaml file.

## Major Version Upgrades: 

The chart does not currently support in-place major version upgrades for existing nodes.

You can use spock to bootstrap upgraded nodes into your cluster, but writes must be stopped on existing nodes during the upgrade process.

See the [Upgrading Postgres](usage/postgres_upgrades.md) for more information.