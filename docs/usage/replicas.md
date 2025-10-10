## Promoting a replica

If you configured a node with more than one instance you can promote one of the replicas like this:

First, list the instances with `kubectl cnpg status <NODE NAME>`:

```
...
Instances status
Name         Current LSN  Replication role  Status  
----         -----------  ----------------  ------  ........
pgedge-n1-1  0/A003490    Primary           OK      ........
pgedge-n1-2  0/A003490    Standby (sync)    OK      ........
pgedge-n1-3  0/A003490    Standby (sync)    OK      ........
...
```

Then, use this command to promote a replica:

```shell
kubectl cnpg promote pgedge-n1 pgedge-n1-2
```
