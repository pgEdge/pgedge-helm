# pgedge

pgEdge is fully distributed PostgreSQL with multi-active replication. This
chart installs pgEdge Distributed Postgres as a StatefulSet.

![Version: 0.0.2](https://img.shields.io/badge/Version-0.0.2-informational?style=flat-square)

## Commonly used values

Most users will want to specify a database name and user names:

```yaml
pgEdge:
  dbSpec:
    dbName: my_application
    users:
      - username: my_application
        superuser: false
        service: postgres
        type: application
      - username: admin
        # Note this user will only have a subset of superuser privileges to exclude the abilities to
        # read files and execute programs on the host.
        superuser: true
        service: postgres
        type: admin
```

> [!WARNING]
> Do not update database users via the Helm chart after they're created. Instead, you should either
> modify the `pgedge-users` secret directly or create a new secret with updated values and specify
> it via the `pgEdge.existingUsersSecret` value. See the [Limitations](#limitations) section below
> for additional caveats on user management.

## Examples

See the [examples README](./examples/README.md) for instructions to try this chart using local
Kubernetes clusters created with [`kind`](https://kind.sigs.k8s.io/).

## Limitations

Most of the pgEdge cluster configuration is set when it's first initialized, and further changes to
the configuration are not read. This behavior affects two main areas:

- Horizontal scaling
  - Increasing the number of nodes in the cluster beyond its initial configuration requires
    additional work to notify the existing pgEdge nodes about the new pgEdge nodes.
- User management
  - After initialization, the only user that's read from the users secret at startup is the internal
    `pgedge` user.
  - New users can be created via SQL, but note that they must be created on every pgEdge separately.
  - Similarly, credential rotation must be performed on each pgEdge node individually. In the case
    of the `pgedge` user, the password in `pgedge-users` secret should also be updated.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| annotations | object | `{}` | Additional annotations to apply to all created objects. |
| labels | object | `{}` | Additional labels to apply to all created objects. |
| pgEdge.appName | string | `"pgedge"` | Determines the name of resources in the pgEdge cluster. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.clusterSpec | object | `{"bootstrap":{"initdb":{"database":"app","encoding":"UTF8","owner":"app","postInitApplicationSQL":["CREATE EXTENSION spock;","CREATE EXTENSION snowflake;","CREATE EXTENSION lolor;"],"postInitSQL":[],"postInitTemplateSQL":[]}},"certificates":{"clientCASecret":"client-ca-key-pair","replicationTLSSecret":"streaming-replica-client-cert"},"imageName":"ghcr.io/pgedge/pgedge-postgres:17-spock5-standard","imagePullPolicy":"Always","instances":1,"managed":{"roles":[{"comment":"Admin role","ensure":"present","login":true,"name":"admin","superuser":true}]},"maxSyncReplicas":0,"minSyncReplicas":0,"postgresql":{"parameters":{"checkpoint_completion_target":"0.9","checkpoint_timeout":"15min","dynamic_shared_memory_type":"posix","hot_standby_feedback":"on","spock.allow_ddl_from_functions":"on","spock.conflict_log_level":"DEBUG","spock.conflict_resolution":"last_update_wins","spock.enable_ddl_replication":"on","spock.include_ddl_repset":"on","spock.save_resolutions":"on","sync_replication_slots":"on","track_commit_timestamp":"on","track_io_timing":"on","wal_level":"logical","wal_sender_timeout":"5s"},"pg_hba":["hostssl app pgedge 0.0.0.0/0 cert","hostssl app admin 0.0.0.0/0 cert","hostssl all streaming_replica all cert map=cnpg_streaming_replica"],"shared_preload_libraries":["pg_stat_statements","snowflake","spock"]},"projectedVolumeTemplate":{"sources":[{"secret":{"items":[{"key":"tls.crt","mode":384,"path":"pgedge/certificates/tls.crt"},{"key":"tls.key","mode":384,"path":"pgedge/certificates/tls.key"},{"key":"ca.crt","mode":384,"path":"pgedge/certificates/ca.crt"}],"name":"pgedge-client-cert"}}]}}` | Default CNPG Cluster specification applied to all nodes, which can be overridden on a per-node basis using the `clusterSpec` field in each node definition. |
| pgEdge.externalNodes | list | `[]` | Configuration for nodes that are part of the pgEdge cluster, but managed externally to this Helm chart. This can be leverage for multi-cluster deployments or to wire up existing CNPG Clusters to a pgEdge cluster. |
| pgEdge.initSpock | bool | `true` | Whether or not to run the init spock job to initialize the pgEdge nodes and subscriptions In multi-cluster deployments, this should only be set to true on the last cluster to be deployed. |
| pgEdge.nodes | list | `[]` | Configuration for each node in the cluster. Each node will be deployed as a separate CNPG Cluster. |
| pgEdge.provisionCerts | bool | `true` | - Whether to deploy cert-manager to manage TLS certificates for the cluster. If false, you must provide your own TLS certificates by creating the secrets defined in `clusterSpec.certificates.clientCASecret` and `clusterSpec.certificates.replicationTLSSecret`. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
