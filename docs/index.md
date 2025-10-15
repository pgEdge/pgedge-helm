# pgedge-helm

This chart installs pgEdge Distributed Postgres using CloudNativePG to manage each node.

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