# Limitations

The following limitations apply when deploying `pgedge-helm`.

## Certificate Management

The chart creates a self-signed CA and issues certificates for managed users on each node, but it does not issue or configure server certificates.

You may manage server certificates yourself using cert-manager and CloudNativePG, but the chart may require modification to use verify-full mode for client connections.

## Repset snapshot is in-memory during reset

When the init-spock job resets a node, it snapshots the replication set configuration into memory, drops the Spock extension with `CASCADE`, recreates it, and then restores the snapshot. If the job crashes or is OOM-killed between the drop and restore steps, the repset configuration is lost and must be manually recreated. The failure window is narrow (a few SQL statements), but operators should be aware of it when troubleshooting failed reset runs.

## App database name

The app database is named "app" by default, and cannot be renamed without modifying various Helm templates and the default values.yaml file.

## Installing multiple times into the same namespace

Installing the chart multiple times into the same Kubernetes namespace is not recommended. Each release expects to manage its own set of resources, and resource name conflicts or unexpected behavior may occur if multiple releases are installed in the same namespace.

For testing purposes, you can modify the `appName` property to separate most chart resources across multiple helm deployments in the same namespace. However, certificates deployed when `provisionCerts` is set to `true` will have the same name across all deployments, and these will conflict in the same namespace.