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
| global.clusterDomain | string | `"cluster.local"` | Set to the cluster's domain if the cluster uses a custom domain. |
| labels | object | `{}` | Additional labels to apply to all created objects. |
| pgEdge.appName | string | `"pgedge"` | Determines the name of the pgEdge StatefulSet and theapp.kubernetes.io/name label. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.clusterSpec.affinity.nodeSelector | object | `{}` |  |
| pgEdge.clusterSpec.bootstrap | object | `{"initdb":{"database":"app","encoding":"UTF8","owner":"app","postInitApplicationSQL":["CREATE EXTENSION spock;","CREATE EXTENSION snowflake;","CREATE EXTENSION lolor;"],"postInitSQL":[],"postInitTemplateSQL":[]}}` | initdb bootstrap configuration for each CNPG Cluster. |
| pgEdge.clusterSpec.instances | int | `1` | The number of PostgreSQL instances to deploy in each CNPG Cluster |
| pgEdge.clusterSpec.postgresql | object | `{"parameters":{"checkpoint_completion_target":"0.9","checkpoint_timeout":"15min","dynamic_shared_memory_type":"posix","hot_standby_feedback":"on","spock.allow_ddl_from_functions":"on","spock.conflict_log_level":"DEBUG","spock.conflict_resolution":"last_update_wins","spock.enable_ddl_replication":"on","spock.include_ddl_repset":"on","spock.save_resolutions":"on","track_commit_timestamp":"on","track_io_timing":"on","wal_level":"logical","wal_sender_timeout":"5s"},"shared_preload_libraries":["pg_stat_statements","snowflake","spock"]}` | PostgreSQL configuration parameters to set for each CNPG cluster. These parameters will override the defaults defined in the values.yaml file. See https://www.postgresql.org/docs/current/runtime-config.html for a list of available parameters. |
| pgEdge.dbSpec.nodes | list | `[]` | Used to override the nodes in the generated db spec. This can be useful in multi-cluster setups, like the included multi-cluster example. |
| pgEdge.dbSpec.users | list | `[{"superuser":false,"type":"application","username":"app"},{"superuser":true,"username":"admin"}]` | Database users to be created. |
| pgEdge.existingUsersSecret | string | `""` | The name of an existing users secret in the release namespace. If not specified, a new secret will generate random passwords for each user and store them in a new secret. See the pgedge-docker README for the format of this secret: https://github.com/pgEdge/pgedge-docker?tab=readme-ov-file#database-configuration |
| pgEdge.nodes | list | `[]` |  |
| pgEdge.pgMajorVersion | int | `17` | Sets the major version of PostgreSQL to use. Must be one of the major versions defined in the   included ImageCatalog. The default is 17, which uses PostgreSQL 17 with pgEdge extensions. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
