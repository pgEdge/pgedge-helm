# pgEdge Helm

The [**pgEdge Helm**](https://github.com/pgedge/pgedge-helm) chart supports deploying both pgEdge Enterprise Postgres and pgEdge Distributed Postgres in Kubernetes. 

This chart leverages [CloudNativePG](https://cloudnative-pg.io/) to manage Postgres, providing flexible options for single-region and multi-region deployments.

At a high level, this chart features support for:

- Postgres 16, 17, and 18 via [pgEdge Enterprise Postgres Images](https://github.com/pgEdge/postgres-images).
- Flexible deployment options for both single-region and multi-region deployments
    - Deploy pgEdge Enterprise Postgres in a single region with optional standby replicas.
    - Deploy pgEdge Distributed Postgres across multiple regions with Spock active-active replication.
- Configuring Spock replication configuration across all nodes during helm install and upgrade processes.
- Best practice configuration defaults for deploying pgEdge Distributed Postgres in Kubernetes.
- Extending / overriding configuration for CloudNativePG across all nodes, or on specific nodes.
- Configuring standby instances with automatic failover, leveraging Spock's delayed feedback and failover slots worker to maintain active-active replication across failovers and promotions.
- Adding pgEdge nodes using Spock or CloudNativePG's bootstrap capabilities to synchronize data from existing nodes or backups.
- Performing Postgres major and minor version upgrades.
- Client certificate authentication for managed users, including the `pgedge` replication user.
- Configuration options to support deployments across multiple Kubernetes clusters.