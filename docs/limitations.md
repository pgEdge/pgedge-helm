# Limitations

The following limitations apply when deploying `pgedge-helm`.

## Certificate Management

The chart creates a self-signed CA and issues certificates for managed users on each node, but it does not issue or configure server certificates.

You may manage server certificates yourself using cert-manager and CloudNativePG, but the chart may require modification to use verify-full mode for client connections.

## Repset snapshot is in-memory during reset

When the init-spock job resets a node, it snapshots the replication set configuration into memory, drops the Spock extension with `CASCADE`, recreates it, and then restores the snapshot. If the job crashes or is OOM-killed between the drop and restore steps, the repset configuration is lost and must be manually recreated. The failure window is narrow (a few SQL statements), but operators should be aware of it when troubleshooting failed reset runs.

## Data written while a replication slot is lost

When Postgres invalidates a Spock replication slot, the `init-spock` job recreates the affected subscription and its slot on the next `helm upgrade` (see [Recovering Lost Replication Slots](usage/monitoring.md#recovering-lost-replication-slots)). The new subscription starts from the current position and does not copy existing data, so rows written on the provider between the invalidation and the upgrade never reach that subscriber. You must reconcile them yourself, for example by comparing the tables on both nodes.

With the default settings, `max_slot_wal_keep_size` is `-1` and `idle_replication_slot_timeout` is `0`, so Postgres does not invalidate slots; the trade-off is that WAL is kept without limit while a subscriber is down, and the volume can fill. If you set `max_slot_wal_keep_size` to bound that, make it large enough to hold the WAL for the longest subscriber outage you expect, and make sure the volume has room for it:

```yaml
pgEdge:
  clusterSpec:
    postgresql:
      parameters:
        max_slot_wal_keep_size: "10GB"
```

## Installing multiple times into the same namespace

Installing the chart multiple times into the same Kubernetes namespace is not recommended. Each release expects to manage its own set of resources, and resource name conflicts or unexpected behavior may occur if multiple releases are installed in the same namespace.

For testing purposes, you can modify the `appName` property to separate most chart resources across multiple helm deployments in the same namespace. However, certificates deployed when `provisionCerts` is set to `true` will have the same name across all deployments, and these will conflict in the same namespace.