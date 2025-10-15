# Monitoring

You can use the `kubectl cnpg` plugin to monitor the CloudNativePG Cluster operating each pgEdge node.

## Checking Status

You can view the status of each node's CloudNativePG Cluster by using the `kubectl cnpg` plugin.

```shell
kubectl cnpg status pgedge-n1 -v
```

This command provides a detailed summary, including cluster health, replication slot status, backup configuration, instance roles, and more. Review the output to ensure all nodes are healthy and replication is functioning as expected.

The section labeled "Unmanaged Replication Slot Status" shows the configured Spock replication slots.

```shell
➜ kubectl cnpg status pgedge-n1 -v
Cluster Summary
Name                 default/pgedge-n1
System ID:           7561064585676382226
PostgreSQL Image:    ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
Primary instance:    pgedge-n1-1
Primary start time:  2025-10-14 13:12:30 +0000 UTC (uptime 6h47m46s)
Status:              Cluster in healthy state
Instances:           3
Ready instances:     3
Size:                608M
Current Write LSN:   0/57000B60 (Timeline: 1 - WAL File: 000000010000000000000057)

Continuous Backup status
Not configured

Physical backups
No running physical backups found

Streaming Replication status
Replication Slots Enabled
Name         Sent LSN    Write LSN   Flush LSN   Replay LSN  Write Lag        Flush Lag        Replay Lag       State      Sync State  Sync Priority  Replication Slot  Slot Restart LSN  Slot WAL Status  Slot Safe WAL Size
----         --------    ---------   ---------   ----------  ---------        ---------        ----------       -----      ----------  -------------  ----------------  ----------------  ---------------  ------------------
pgedge-n1-2  0/57000B60  0/57000B60  0/57000B60  0/57000B60  00:00:00.000703  00:00:00.002004  00:00:00.002012  streaming  quorum      1              active            0/57000B60        reserved         NULL
pgedge-n1-3  0/57000B60  0/57000B60  0/57000B60  0/57000B60  00:00:00.000969  00:00:00.004404  00:00:00.004434  streaming  quorum      1              active            0/57000B60        reserved         NULL

Unmanaged Replication Slot Status
Slot Name             Slot Type  Database  Active  Restart LSN  XMin  Catalog XMin  Datoid  Plugin        Wal Status  Safe Wal Size
---------             ---------  --------  ------  -----------  ----  ------------  ------  ------        ----------  -------------
spk_app_n1_sub_n1_n2  logical    app       true    0/57000B28         15182         16385   spock_output  reserved    NULL
spk_app_n1_sub_n1_n3  logical    app       true    0/57000B28         15182         16385   spock_output  reserved    NULL

Managed roles status
Status       Roles
------       -----
not-managed  app
reconciled   admin
reserved     postgres,streaming_replica

Tablespaces status
No managed tablespaces

Pod Disruption Budgets status
Name               Role     Expected Pods  Current Healthy  Minimum Desired Healthy  Disruptions Allowed
----               ----     -------------  ---------------  -----------------------  -------------------
pgedge-n1          replica  2              2                1                        1
pgedge-n1-primary  primary  1              1                1                        0

Instances status
Name         Current LSN  Replication role  Status  QoS         Manager Version  Node
----         -----------  ----------------  ------  ---         ---------------  ----
pgedge-n1-1  0/57000B60   Primary           OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000000
pgedge-n1-2  0/57000C18   Standby (sync)    OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000002
pgedge-n1-3  0/57000C18   Standby (sync)    OK      BestEffort  1.27.0           aks-agentpool-14750958-vmss000001

Plugins status
No plugins found
```

## Viewing Logs

You can view Postgres logs for a specific node using the `kubectl cnpg` plugin. Replace `pgedge-n1` with the name of your node to display and format the logs for easier reading:

```shell
➜ kubectl cnpg logs cluster pgedge-n1 | kubectl cnpg logs pretty

2025-10-14T13:14:26.771 INFO     pgedge-n1-1 postgres         starting spock database manager for database app
2025-10-14T13:14:26.945 INFO     pgedge-n1-1 postgres         logical decoding found consistent point at 0/604E640
2025-10-14T13:14:26.945 INFO     pgedge-n1-1 postgres         exported logical decoding snapshot: "00000092-00000002-1" with 0 transaction IDs
2025-10-14T13:14:26.974 INFO     pgedge-n1-1 postgres         starting logical decoding for slot "spk_app_n1_sub_n1_n2"
2025-10-14T13:14:26.974 INFO     pgedge-n1-1 postgres         logical decoding found consistent point at 0/604E640
2025-10-14T13:14:27.015 INFO     pgedge-n1-1 postgres         manager worker [936] at slot 1 generation 1 detaching cleanly
2025-10-14T13:14:27.019 INFO     pgedge-n1-1 postgres         logical decoding found consistent point at 0/604E678
2025-10-14T13:14:27.020 INFO     pgedge-n1-1 postgres         exported logical decoding snapshot: "00000089-00000003-1" with 0 transaction IDs
2025-10-14T13:14:27.055 INFO     pgedge-n1-1 postgres         starting logical decoding for slot "spk_app_n1_sub_n1_n3"
2025-10-14T13:14:27.055 INFO     pgedge-n1-1 postgres         logical decoding found consistent point at 0/604E678
2025-10-14T13:14:27.111 INFO     pgedge-n1-1 postgres         SPOCK sub_n2_n1: connected
2025-10-14T13:14:28.055 INFO     pgedge-n1-1 postgres         manager worker [943] at slot 3 generation 1 detaching cleanly
2025-10-14T13:14:28.060 INFO     pgedge-n1-1 postgres         manager worker [944] at slot 3 generation 2 detaching cleanly
2025-10-14T13:14:28.143 INFO     pgedge-n1-1 postgres         SPOCK sub_n3_n1: connected
2025-10-14T13:15:16.598 INFO     pgedge-n1-3 postgres         starting spock_failover_slots replica worker
2025-10-14T13:15:16.631 INFO     pgedge-n1-3 postgres         waiting for remote slot spk_app_n1_sub_n1_n2 lsn (0/6050CC0) and catalog xmin (793) to pass local sl...
2025-10-14T13:15:23.476 INFO     pgedge-n1-2 postgres         waiting for remote slot spk_app_n1_sub_n1_n2 lsn (0/6053780) and catalog xmin (807) to pass local sl...
2025-10-14T13:15:26.700 INFO     pgedge-n1-3 postgres         waiting for remote slot spk_app_n1_sub_n1_n3 lsn (0/6050CC0) and catalog xmin (793) to pass local sl...
2025-10-14T13:15:43.553 INFO     pgedge-n1-2 postgres         waiting for remote slot spk_app_n1_sub_n1_n3 lsn (0/6053780) and catalog xmin (807) to pass local sl...
2025-10-14T13:28:22.134 INFO     pgedge-n1-2 postgres         restartpoint starting: time
2025-10-14T13:28:34.272 INFO     pgedge-n1-2 postgres         restartpoint complete: wrote 124 buffers (0.8%); 0 WAL file(s) added, 0 removed, 0 recycled; write=1...
```
