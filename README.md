# pgEdge Distributed Postgres Helm Chart

This chart installs pgEdge Distributed Postgres using CloudNativePG to manage each node.

![Version: 0.0.3](https://img.shields.io/badge/Version-0.0.3-informational?style=flat-square)

## Features

At a high level, this chart features support for:

- Postgres 16, 17, and 18 via [pgEdge Enterprise Postgres Images](https://github.com/pgEdge/postgres-images).
- Configuring Spock replication configuration across all nodes during helm install and upgrade processes.
- Best practice configuration defaults for deploying pgEdge Distributed Postgres in Kubernetes.
- Extending / overriding configuration for CloudNativePG across all nodes, or on specific nodes.
- Configuring standby instances with automatic failover, leveraging Spock's delayed feedback and failover slots worker to maintain multi-active replication across failovers and promotions.
- Adding pgEdge nodes using Spock or CloudNativePG's bootstrap capabilities to synchronize data from existing nodes or backups.
- Performing Postgres major and minor version upgrades.
- Client certificate authentication for managed users, including the `pgedge` replication user.
- Configuration options to support multi-cluster deployments.

## Prerequisites

In order for this chart to work, you must pre-install two operators into your Kubernetes clusters:

- [CloudNativePG](https://cloudnative-pg.io/)
- [cert-manager](https://cert-manager.io/)

## Documentation

The documentation for this chart uses MkDocs with the Material theme to generate styled static HTML documentation from Markdown files in the docs directory.

The documentation can be accessed locally at http://localhost:8000 using:

```shell
make docs
```

### helm-docs

[helm-docs](https://github.com/norwoodj/helm-docs) is used to generate values.yaml reference documentation dynamically from `values.yaml`.

This is in use in the following files:

- README.md.gotmpl
  - generates README.md
- docs/configuration.md.gotmpl
  - generates docs/configuration.md

You can run `make gen-docs` after updating the templates to generate the associated markdown file.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| pgEdge.appName | string | `"pgedge"` | Determines the name of resources in the pgEdge cluster. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.clusterSpec | object | `{"bootstrap":{"initdb":{"database":"app","encoding":"UTF8","owner":"app","postInitApplicationSQL":["CREATE EXTENSION spock;"],"postInitSQL":[],"postInitTemplateSQL":[]}},"certificates":{"clientCASecret":"client-ca-key-pair","replicationTLSSecret":"streaming-replica-client-cert"},"imageName":"ghcr.io/pgedge/pgedge-postgres:17-spock5-standard","imagePullPolicy":"Always","instances":1,"managed":{"roles":[{"comment":"Admin role","ensure":"present","login":true,"name":"admin","superuser":true}]},"postgresql":{"parameters":{"checkpoint_completion_target":"0.9","checkpoint_timeout":"15min","dynamic_shared_memory_type":"posix","hot_standby_feedback":"on","spock.allow_ddl_from_functions":"on","spock.conflict_log_level":"DEBUG","spock.conflict_resolution":"last_update_wins","spock.enable_ddl_replication":"on","spock.include_ddl_repset":"on","spock.save_resolutions":"on","track_commit_timestamp":"on","track_io_timing":"on","wal_level":"logical","wal_sender_timeout":"5s"},"pg_hba":["hostssl app pgedge 0.0.0.0/0 cert","hostssl app admin 0.0.0.0/0 cert","hostssl app app 0.0.0.0/0 cert","hostssl all streaming_replica all cert map=cnpg_streaming_replica"],"pg_ident":["local postgres admin","local postgres app"],"shared_preload_libraries":["pg_stat_statements","snowflake","spock"]},"projectedVolumeTemplate":{"sources":[{"secret":{"items":[{"key":"tls.crt","mode":384,"path":"pgedge/certificates/tls.crt"},{"key":"tls.key","mode":384,"path":"pgedge/certificates/tls.key"},{"key":"ca.crt","mode":384,"path":"pgedge/certificates/ca.crt"}],"name":"pgedge-client-cert"}}]}}` | Default CloudNativePG Cluster specification applied to all nodes, which can be overridden on a per-node basis using the `clusterSpec` field in each node definition. |
| pgEdge.externalNodes | list | `[]` | Configuration for nodes that are part of the pgEdge cluster, but managed externally to this Helm chart. This can be leverage for multi-cluster deployments or to wire up existing CloudNativePG Clusters to a pgEdge cluster. |
| pgEdge.initSpock | bool | `true` | Whether or not to run the init-spock job to initialize the pgEdge nodes and subscriptions In multi-cluster deployments, this should only be set to true on the last cluster to be deployed. |
| pgEdge.nodes | list | `[]` | Configuration for each node in the pgEdge cluster. Each node will be deployed as a separate CloudNativePG Cluster. |
| pgEdge.provisionCerts | bool | `true` | Whether to deploy cert-manager to manage TLS certificates for the cluster. If false, you must provide your own TLS certificates by creating the secrets defined in `clusterSpec.certificates.clientCASecret` and `clusterSpec.certificates.replicationTLSSecret`. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
