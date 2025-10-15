# pgedge-helm

This chart installs pgEdge Distributed Postgres using CloudNativePG to manage each node.

At a high level, this chart features:

- Support for Postgres 16, 17, and 18 via [pgEdge Enterprise Postgres Images](https://github.com/pgEdge/postgres-images)
- Support for configuring Spock replication across all nodes during helm install and upgrade
- Best practice configuration defaults for deploying pgEdge Distributed Postgres in Kubernetes
- Ability to extend / override configuration for CloudNativePG across all nodes, or on specific nodes
- Ability to configure standby instances with automatic failover, leveraging spock's delayed feedback and failover slots worker to maintain multi-active replication across failovers / promotions
- Ability to add pgEdge nodes using Spock or CloudNativePG's bootstrap capabilities to synchronize data from existing nodes or backups
- Ability to perform Postgres major and minor version upgrades for Postgres
- Client certificate authentication for managed users, including the pgedge replication user
- Configuration mechanisms to support multi-cluster deployments