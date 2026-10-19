# Configuring Standby Instances

This chart supports the ability to configure standby instances with automatic failover, leveraging spock's delayed feedback and failover slots worker to maintain multi-active replication across failovers / promotions.

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

## Spock 6 and standby instances

On PostgreSQL 16 or an older version, Spock 6 uses the same failover-slots
worker as Spock 5. On PostgreSQL 17 and 18, this chart instead uses
PostgreSQL's own native logical-slot synchronization — on 18, Spock 6 no
longer runs its failover-slots worker at all, and this chart opts 17 into
the same native path too, for consistent behavior across both.

**This is handled automatically.** `pgEdge.nativeSlotSync` (default `auto`)
reads each node's `imageName` tag (e.g. `18-spock6-standard`) and enables
native slot sync whenever it's Spock 6+ and PostgreSQL 17+. You don't need
to set anything yourself in the normal case. It falls back to off if a
node's image tag doesn't follow that convention (a digest reference or a
custom tag), since turning it on for an unsupported Postgres version
prevents that node from starting — set `pgEdge.nativeSlotSync: true` to
force it on for such a node, or `false` to force it off. The `init-spock`
job separately verifies this is on wherever it's actually required (Spock
6+, PostgreSQL 17+, `instances > 1`) and fails with a clear error if not, so
a tag auto-detection can't parse doesn't silently leave a cluster
unprotected.

When this chart pre-creates a replication slot for a new node during
add-node (`internal/spock/replication_slot_create.go`), it detects the live
Spock and PostgreSQL versions on that connection and sets the slot's
`FAILOVER` flag to match, under the same condition. No configuration is
needed for this part either.

!!! warning

    A Postgres/Spock-internal race remains: an unplanned failover doesn't
    yet reliably survive on Spock 6, because PostgreSQL's native slot-sync
    worker can refuse to sync a slot right before promotion if the
    standby's catalog xmin has already advanced past what the slot needs,
    leaving the promoted node with no slot. This is untracked upstream as
    of this writing and isn't something this chart's configuration can work
    around.

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
