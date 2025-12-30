# Configuration

This chart is built around managing each pgEdge node as a [CloudNativePG](https://cloudnative-pg.io/) `Cluster`.

The chart contains a default `clusterSpec` in `values.yaml` which defines the required configuration for deploying pgEdge with CloudNativePG, including:

- deploying with the [pgEdge Enterprise Postgres Images](https://github.com/pgedge/postgres-images).
- loading and initializing required extensions for pgEdge Distributed Postgres.
- setting up required PostgreSQL configuration parameters.
- configuring client certificate authentication for managed users (app, admin, streaming_replica).
- allowing local connections for the app and admin users for testing / development purposes.

The simplest example values file, which deploys a single primary instance for each node, looks like this:

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
    storage:
      size: 1Gi
```

As shown, The default `clusterSpec` can be overridden for all nodes with specific configuration required for your Kubernetes setup.

You can also override the `clusterSpec` for specific nodes if you require more granular control.

For example, to create a 3-node cluster with 3 instances on node `n1` and single instances on nodes `n2` and `n3`, you could use:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec:
        instances: 3
        postgresql:
          synchronous:
            method: any
            number: 1
            dataDurability: required
    - name: n2
      hostname: pgedge-n2-rw
    - name: n3
      hostname: pgedge-n3-rw
```

This override behavior is enabled via `mergeOverwrite` in Helm. You should be mindful that lists are replaced, not merged.

If you override a list in the `clusterSpec` for a node, you must include all required elements in that list, pulling from the values file example, and using `helm template` to verify your configuration.

For more information about configuring CloudNativePG, see the [CloudNativePG documentation](https://cloudnative-pg.io/docs/).

## Spock configuration

This chart contains a python job to initialize Spock multi-master replication across all nodes once they are all available.

This job runs by default, waiting for any clusters associated with the current deployment to be ready before performing initialization.

If you wish to disable this behavior, you can set `pgEdge.initSpock` to `false`.

### snowflake.node and lolor.node

This chart automatically configures `snowflake.node` and `lolor.node` based on the `name` property of each node.

For example, a node named `n1` will have the following Postgres configuration applied to the node to ensure snowflake and lolor are configured appropriately:

```yaml
  postgresql:
    parameters:
      lolor.node: "1"
      snowflake.node: "1"
```

If you wish to override this behavior, or plan to utilize alternate naming schemes for your node, you can set the `ordinal` property for each node:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: a
      hostname: pgedge-a-rw
      ordinal: 1
    - name: b
      hostname: pgedge-b-rw
      ordinal: 2
    - name: c
      hostname: pgedge-c-rw
      ordinal: 3
  clusterSpec:
    storage:
      size: 1Gi
```

## Extensions

This chart supports all extensions included in the standard flavor of the [pgEdge Enterprise Postgres Image](https://github.com/pgedge/postgres-images?tab=readme-ov-file#standard-images).

By default, `shared_preload_libraries` contains `pg_stat_statements`, `snowflake`, and `spock`. For additional extensions, you may need to override `postgresql.shared_preload_libraries` and set additional parameters in `postgresql.parameters` in your `values.yaml` to ensure the extension is loaded and configured properly.

!!! note

    Always include `spock` in `shared_preload_libraries`, as it is required for core functionality provided by this chart. This chart will call `CREATE EXTENSION` for spock when initializing each CloudNativePG Cluster.

## Values reference

You can customize this Helm chart by specifying configuration parameters in your `values.yaml` file.

The following table lists all available options and their descriptions.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| pgEdge.appName | string | `"pgedge"` | Determines the name of resources in the pgEdge cluster. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.clusterSpec | object | `{"bootstrap":{"initdb":{"database":"app","encoding":"UTF8","owner":"app","postInitApplicationSQL":["CREATE EXTENSION spock;"],"postInitSQL":[],"postInitTemplateSQL":[]}},"certificates":{"clientCASecret":"client-ca-key-pair","replicationTLSSecret":"streaming-replica-client-cert"},"imageName":"ghcr.io/pgedge/pgedge-postgres:17-spock5-standard","imagePullPolicy":"Always","instances":1,"managed":{"roles":[{"comment":"Admin role","ensure":"present","login":true,"name":"admin","superuser":true}]},"postgresql":{"parameters":{"checkpoint_completion_target":"0.9","checkpoint_timeout":"15min","dynamic_shared_memory_type":"posix","hot_standby_feedback":"on","spock.allow_ddl_from_functions":"on","spock.conflict_log_level":"DEBUG","spock.conflict_resolution":"last_update_wins","spock.enable_ddl_replication":"on","spock.include_ddl_repset":"on","spock.save_resolutions":"on","track_commit_timestamp":"on","track_io_timing":"on","wal_level":"logical","wal_sender_timeout":"5s"},"pg_hba":["hostssl app pgedge 0.0.0.0/0 cert","hostssl app admin 0.0.0.0/0 cert","hostssl app app 0.0.0.0/0 cert","hostssl all streaming_replica all cert map=cnpg_streaming_replica"],"pg_ident":["local postgres admin","local postgres app"],"shared_preload_libraries":["pg_stat_statements","snowflake","spock"]},"projectedVolumeTemplate":{"sources":[{"secret":{"items":[{"key":"tls.crt","mode":384,"path":"pgedge/certificates/tls.crt"},{"key":"tls.key","mode":384,"path":"pgedge/certificates/tls.key"},{"key":"ca.crt","mode":384,"path":"pgedge/certificates/ca.crt"}],"name":"pgedge-client-cert"}}]}}` | Default CloudNativePG Cluster specification applied to all nodes, which can be overridden on a per-node basis using the `clusterSpec` field in each node definition. |
| pgEdge.externalNodes | list | `[]` | Configuration for nodes that are part of the pgEdge cluster, but managed externally to this Helm chart. This can be leveraged for multi-cluster deployments or to wire up existing CloudNativePG Clusters to a pgEdge cluster. |
| pgEdge.initSpock.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| pgEdge.initSpock.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| pgEdge.initSpock.containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| pgEdge.initSpock.enabled | bool | `true` | Whether or not to run the init-spock job to initialize the pgEdge nodes and subscriptions In multi-cluster deployments, this should only be set to true on the last cluster to be deployed. |
| pgEdge.initSpock.imageName | string | `"ghcr.io/kevinpthorne/pgedge-helm-utils:v0.1.1"` | Docker image for the init-spock job. This image contains Python and required dependencies for the job to run. This image is versioned alongside the Helm chart. |
| pgEdge.initSpock.podSecurityContext.runAsNonRoot | bool | `true` |  |
| pgEdge.initSpock.podSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| pgEdge.nodes | list | `[]` | Configuration for each node in the pgEdge cluster. Each node will be deployed as a separate CloudNativePG Cluster. |
| pgEdge.provisionCerts | bool | `true` | Whether to deploy cert-manager to manage TLS certificates for the cluster. If false, you must provide your own TLS certificates by creating the secrets defined in `clusterSpec.certificates.clientCASecret` and `clusterSpec.certificates.replicationTLSSecret`. |