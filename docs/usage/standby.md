# Configuring Standby Instances

This chart supports the ability to configure standby instances with automatic failover, leveraging spock's delayed feedback and failover slots worker (or PostgreSQL's native slot synchronization with Spock 6.0 on PostgreSQL 18) to maintain multi-active replication across failovers / promotions.

We recommend deploying at least three instances per node to ensure high availability and fault tolerance. This configuration allows each node to maintain quorum and continue operating even if one instance becomes unavailable. Adjust the number of replicas based on your workload requirements and desired level of redundancy.

An example configuration for a three node cluster, with 3 instances per node, looks like this:

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
    instances: 3
    postgresql:
      synchronous:
        method: any
        number: 1
        dataDurability: required
    storage:
      size: 1Gi
```

## Replication slot readiness

Other nodes replicate from a node through Spock replication slots on its primary. These slots are copied to each standby in one of two ways:

- With Spock 5, and with Spock 6.0 on PostgreSQL 16 and 17, Spock's failover slot worker copies them, every second by default since Spock 5.0.11.
- With Spock 6.0 on PostgreSQL 18, PostgreSQL's native slot synchronization copies them, and the chart turns it on through CloudNativePG. With Spock 6.0 on PostgreSQL 17, native slot synchronization copies them instead of Spock's worker when you turn it on with `pgEdge.nativeSlotSync: true`, which sets `sync_replication_slots` to `on`. See [Using Spock 6.0](spock6.md#standby-instances-and-failover-slots).

Since Spock 5.0.14, a new copy starts out as a temporary slot and only becomes permanent once the primary's slot has advanced past the standby's position. Native slot synchronization behaves the same way. Promotion drops temporary slots, so if a standby is promoted before its copies are permanent, the other nodes stop replicating from this node.

This mainly matters right after a node or standby is created, or after several promotions in quick succession. Before promoting a replica, run this query on the current primary and on the standby you want to promote:

```sql
SELECT slot_name, temporary, synced, wal_status, conflicting
FROM pg_replication_slots
WHERE plugin = 'spock_output'
ORDER BY slot_name;
```

On PostgreSQL 16, leave out the `synced` column, which was added in PostgreSQL 17.

The standby is ready to promote only when, for every slot name returned on the primary, the standby returns a row with the same name where:

- `temporary` is `f`;
- `synced` is `t`, if native slot synchronization copies the slots, that is when `sync_replication_slots` is `on` (always the case for Spock 6.0 on PostgreSQL 18); a copy with `synced` set to `f` there was made by Spock's failover slot worker, for example before an upgrade from Spock 5 or before you turned native slot synchronization on, and it does not advance and blocks native slot synchronization of that slot;
- `wal_status` is not `lost`, meaning the WAL the slot needs has not been removed;
- `conflicting` is not `t`, meaning the slot has not been invalidated by a recovery conflict.

Compare the slot names on both sides, not just the rows on the standby: a slot that has no copy on the standby yet does not appear there at all. Do not promote the standby while any slot is missing or fails one of these checks.

In Spock 5.0.14, a copy can stay temporary on an idle primary even after it has caught up. If that happens, run `SELECT pg_log_standby_snapshot();` on the primary (or any write transaction), then check again. A copy that is lost or conflicting is dropped and recreated from the primary by the failover slot worker or by native slot synchronization, after which it goes through the temporary stage again.

Spock's failover slot worker recreates an invalidated copy only in Spock 5.0.14 and later, and only while `spock.failover_slots_recreate_invalidated` is `on`, which is the default. On older versions, or with that setting off, drop the invalidated copy on the standby with `SELECT pg_drop_replication_slot('<slot name>');`, and the worker copies the slot again.

A copy can only recover if the slot on the primary is usable. If the primary's slot is itself lost or conflicting, neither the failover slot worker nor native slot synchronization replaces the copy, and the other nodes can't replicate from the primary through that slot either. Repair the primary's slot first: drop it on the primary with `pg_drop_replication_slot()`, then run `helm upgrade` with the same values. The init-spock job recreates the subscription that reads the slot, which recreates the slot, as described in [Automatic failover](#automatic-failover), with the same caveat about changes that are not replicated. The standby then copies the new slot.

### Automatic failover

CloudNativePG's automatic failover doesn't perform this check. If the primary fails before the standby's copies are permanent, CloudNativePG still promotes the standby, and the other nodes stop replicating from this node. Their subscriptions to it report errors such as `replication slot "spk_app_n1_sub_n1_n2" does not exist`.

To recover, run `helm upgrade` with the same values. The init-spock job finds each subscription whose slot is missing on the new primary and recreates it, which also recreates the slot. The recreated subscription starts from the new slot's position and does not copy existing data, so changes made on this node between the failover and the recovery are not replicated to the other nodes. Check for and repair any differences between the nodes afterwards.

## Promoting a replica

If you have configured a node with more than one instance, you can perform a promotion using the `kubectl cnpg` plugin.

First, list the instances with `kubectl cnpg status <NODE NAME>`:

```sh
➜  kubectl cnpg status pgedge-n1
Cluster Summary
Name                 default/pgedge-n1
System ID:           7561064585676382226
PostgreSQL Image:    ghcr.io/pgedge/pgedge-postgres:18-spock5-standard
Primary instance:    pgedge-n1-1
Primary start time:  2025-10-14 13:12:30 +0000 UTC (uptime 8h3m57s)
Status:              Cluster in healthy state
Instances:           3
Ready instances:     3
Size:                608M
Current Write LSN:   0/66005850 (Timeline: 1 - WAL File: 000000010000000000000066)

Continuous Backup status
Not configured

Streaming Replication status
Replication Slots Enabled
Name         Sent LSN    Write LSN   Flush LSN   Replay LSN  Write Lag  Flush Lag  Replay Lag  State      Sync State  Sync Priority  Replication Slot
----         --------    ---------   ---------   ----------  ---------  ---------  ----------  -----      ----------  -------------  ----------------
pgedge-n1-2  0/66005850  0/66005850  0/66005850  0/66005850  00:00:00   00:00:00   00:00:00    streaming  quorum      1              active
pgedge-n1-3  0/66005850  0/66005850  0/66005850  0/66005850  00:00:00   00:00:00   00:00:00    streaming  quorum      1              active

Instances status
Name         Current LSN  Replication role  Status  QoS         Manager Version  Node
----         -----------  ----------------  ------  ---         ---------------  ----
pgedge-n1-1  0/66005850   Primary           OK      BestEffort  1.28.0           aks-agentpool-14750958-vmss000000
pgedge-n1-2  0/66005850   Standby (sync)    OK      BestEffort  1.28.0           aks-agentpool-14750958-vmss000002
pgedge-n1-3  0/66005850   Standby (sync)    OK      BestEffort  1.28.0           aks-agentpool-14750958-vmss000001
```

To perform the promotion, use this command to promote the specified replica. For example, to promote `pgedge-n1-2`:

```shell
➜ kubectl cnpg promote pgedge-n1 pgedge-n1-2

{"level":"info","ts":"2025-10-14T16:17:16.217477-05:00","msg":"Cluster has become unhealthy"}
Node pgedge-n1-2 in cluster pgedge-n1 will be promoted
```

You can monitor the status of the promotion using `kubectl cnpg status pgedge-n1`.
