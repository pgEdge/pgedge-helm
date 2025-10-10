# Monitoring

## Checking Status

You can view the status of a cluster with this command. The section labeled "Unmanaged Replication Slot Status" shows the Spock replication slots:

```shell
kubectl cnpg status pgedge-n1 -v
```

## Viewing Logs

Use this command to view logs for specific nodes. Replace `pgedge-n1` with the name of the node. This will use the `cnpg` plugin to format the logs for easy viewing.

```shell
kubectl cnpg logs cluster pgedge-n1 | kubectl cnpg logs pretty
```
