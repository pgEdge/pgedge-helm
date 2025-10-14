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

## Promoting a replica

If you have configured a node with more than one instance, you can perform a promotion using the `kubectl cnpg` plugin.

First, list the instances with `kubectl cnpg status <NODE NAME>`:

```sh
➜  kubectl cnpg status pgedge-n1
Cluster Summary
Name                 default/pgedge-n1
System ID:           7561064585676382226
PostgreSQL Image:    ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
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
pgedge-n1-1  0/66005850   Primary           OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000000
pgedge-n1-2  0/66005850   Standby (sync)    OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000002
pgedge-n1-3  0/66005850   Standby (sync)    OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000001
```

To perform the promotion, use this command to promote the specified replica. For example, to promote `pgedge-n1-2`:

```shell
➜ kubectl cnpg promote pgedge-n1 pgedge-n1-2

{"level":"info","ts":"2025-10-14T16:17:16.217477-05:00","msg":"Cluster has become unhealthy"}
Node pgedge-n1-2 in cluster pgedge-n1 will be promoted
```

You can monitor the status of the promotion using `kubectl cnpg status pgedge-n1`.
