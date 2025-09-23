# pgEdge Distributed Postgres Helm Chart

This chart installs pgEdge Distributed Postgres using CloudNativePG to manage each node.

![Version: 0.0.3-beta1](https://img.shields.io/badge/Version-0.0.3--beta1-informational?style=flat-square)

## Prerequisites

In order for this chart to work, you must pre-install two operators into your Kubernetes clusters:

- [CloudNativePG](https://cloudnative-pg.io/)
- [cert-manager](https://cert-manager.io/)

## Installation & Usage

See the [Installation and Usage Guide](./docs/installation-usage.md) for installation and setup instructions for deploying this chart.

## Configuration

This chart is built around managing each pgEdge node as a [CloudNativePG](https://cloudnative-pg.io/) `Cluster`.

The chart contains a default `clusterSpec` in `values.yaml` which sets up required configuration for pgEdge, including:

- Using the [pgEdge Enterprise Postgres Image](https://github.com/pgedge/postgres-images)
- Loading and initializing required extensions for pgEdge Distributed Postgres
- Setting up required PostgreSQL configuration parameters
- Configuring client certificate authentication for managed users (app, admin, streaming_replica)
- Allowing local connections for the `app` and `admin` users for testing / development purposes

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

In addition, you can override the `clusterSpec` for individual nodes.

For example, to create a 3-node cluster with 3 instances on node `n1` and single instances on nodes `n2` and `n3`, you could use:

```yaml
pgEdge:
  appName: pgedge
  nodes:
    - name: n1
      hostname: pgedge-n1-rw
      clusterSpec:
        instances: 3
        minSyncReplicas: 1
        maxSyncReplicas: 2
    - name: n2
      hostname: pgedge-n2-rw
    - name: n3
      hostname: pgedge-n3-rw
```

This override behavior is enabled via `mergeOverwrite` in Helm. You should be mindful that lists are replaced, not merged.

If you override a list in the `clusterSpec` for a node, you must include all required elements in that list, pulling from the values file example, and using `helm template` to verify your configuration.

For more information on configuring CloudNativePG, be sure to reference their documentation at https://cloudnative-pg.io/docs/

## Certificates

This chart leverages `cert-manager` to create a self-signed CA and issue client certificates for a set of managed users on each node:

- `app`: a user for application connections
- `admin`: a superuser for administrative purposes
- `pgedge`: used for multi-active replication connections between nodes
- `streaming_replica`: used for physical replication connections between nodes

This makes it easier to get started with pgEdge, but you may want to use your own CA in production.

In that case, you can disable the self-signed CA creation and certificate issuance by setting
`pgEdge.provisionCerts` to `false`, and issuing your own certificates using cert-manager or another tool.

These can then be plugged into your clusterSpec accordingly:

```yaml
pgEdge:
  appName: pgedge
  provisionCerts: false
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
    certificates:
        clientCASecret: <secret-containing-client-ca>
        replicationTLSSecret: <secret-containing-replication-tls-cert>
```

## Spock initialization

This chart contains a python job to initialize spock multi-master replication across all nodes once they are all available.

This job runs by default, waiting for any clusters within the namespace to be ready before performing initialization.

If you wish to disable this behavior, you can set `pgEdge.initSpock` to `false`.

## Multi-cluster considerations

If you are deploying nodes across multiple Kubernetes clusters, you must take additional steps to leverage this chart, since Helm is not designed to manage resources across multiple clusters by default.

This includes:

1. Installing the required operators (CloudNativePG and cert-manager) into each Kubernetes cluster.
2. Creating required certificates for managed users (app, admin, pgedge, streaming_replica) in each Kubernetes cluster.
3. Installing the chart into each Kubernetes cluster separately, with the following configuration:
  a. Setting `pgEdge.initSpock` to `false` to defer spock initialization until a later deployment.
  b. Setting the `pgEdge.nodes` property to only list nodes which should be deployed in a particular cluster.
4. Exposing the read/write services for each node across clusters using Kubernetes network tools like Cilium or Submariner to enable cross-cluster DNS and connectivity.
5. Installing the chart into one of the clusters with all nodes listed under `pgEdge.externalNodes` and `pgEdge.initSpock` set to `true` to initialize spock configuration.

## Limitations

- Certificate Management: The chart creates a self-signed CA and issues certificates for managed users on each node, but it does not issue or configure server certificates.
  - You may manage server certificates yourself using cert-manager and CloudNativePG, but the chart may require modification to use verify-full mode for client connections.
- Node Management: Adding nodes is supported by updating the `pgEdge.nodes` value and running a helm upgrade, but it does not load existing data into the new node.
  - This operation currently requires that you bootstrap the new node using CloudNativePG's [Bootstrap from another cluster](https://cloudnative-pg.io/documentation/1.27/bootstrap/#bootstrap-from-another-cluster) capability.
  - See the [Installation and Usage Guide](./docs/installation-usage.md) for examples.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| pgEdge.appName | string | `"pgedge"` | Determines the name of resources in the pgEdge cluster. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.clusterSpec | object | `{"bootstrap":{"initdb":{"database":"app","encoding":"UTF8","owner":"app","postInitApplicationSQL":["CREATE EXTENSION spock;","CREATE EXTENSION snowflake;","CREATE EXTENSION lolor;"],"postInitSQL":[],"postInitTemplateSQL":[]}},"certificates":{"clientCASecret":"client-ca-key-pair","replicationTLSSecret":"streaming-replica-client-cert"},"imageName":"ghcr.io/pgedge/pgedge-postgres:17-spock5-standard","imagePullPolicy":"Always","instances":1,"managed":{"roles":[{"comment":"Admin role","ensure":"present","login":true,"name":"admin","superuser":true}]},"maxSyncReplicas":0,"minSyncReplicas":0,"postgresql":{"parameters":{"checkpoint_completion_target":"0.9","checkpoint_timeout":"15min","dynamic_shared_memory_type":"posix","hot_standby_feedback":"on","spock.allow_ddl_from_functions":"on","spock.conflict_log_level":"DEBUG","spock.conflict_resolution":"last_update_wins","spock.enable_ddl_replication":"on","spock.include_ddl_repset":"on","spock.save_resolutions":"on","sync_replication_slots":"on","track_commit_timestamp":"on","track_io_timing":"on","wal_level":"logical","wal_sender_timeout":"5s"},"pg_hba":["hostssl app pgedge 0.0.0.0/0 cert","hostssl app admin 0.0.0.0/0 cert","hostssl app app 0.0.0.0/0 cert","hostssl all streaming_replica all cert map=cnpg_streaming_replica"],"pg_ident":["local postgres admin","local postgres app"],"shared_preload_libraries":["pg_stat_statements","snowflake","spock"]},"projectedVolumeTemplate":{"sources":[{"secret":{"items":[{"key":"tls.crt","mode":384,"path":"pgedge/certificates/tls.crt"},{"key":"tls.key","mode":384,"path":"pgedge/certificates/tls.key"},{"key":"ca.crt","mode":384,"path":"pgedge/certificates/ca.crt"}],"name":"pgedge-client-cert"}}]}}` | Default CloudNativePG Cluster specification applied to all nodes, which can be overridden on a per-node basis using the `clusterSpec` field in each node definition. |
| pgEdge.externalNodes | list | `[]` | Configuration for nodes that are part of the pgEdge cluster, but managed externally to this Helm chart. This can be leverage for multi-cluster deployments or to wire up existing CloudNativePG Clusters to a pgEdge cluster. |
| pgEdge.initSpock | bool | `true` | Whether or not to run the init spock job to initialize the pgEdge nodes and subscriptions In multi-cluster deployments, this should only be set to true on the last cluster to be deployed. |
| pgEdge.nodes | list | `[]` | Configuration for each node in the pgEdge cluster. Each node will be deployed as a separate CloudNativePG Cluster. |
| pgEdge.provisionCerts | bool | `true` | Whether to deploy cert-manager to manage TLS certificates for the cluster. If false, you must provide your own TLS certificates by creating the secrets defined in `clusterSpec.certificates.clientCASecret` and `clusterSpec.certificates.replicationTLSSecret`. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
