# pgEdge Distributed Postgres Helm Chart

This chart installs pgEdge Distributed Postgres using CloudNativePG to manage each node.

- Best practice configuration defaults for deploying pgEdge Distributed Postgres in Kubernetes
- Support for configuring Spock replication across all nodes during Helm install and upgrade
- Ability to extend or override configuration to pass through settings to CloudNativePG for all nodes or specific nodes
- Leverage CloudNativePG features via pass-through configuration
- Add pgEdge nodes using Spock or CloudNativePG’s bootstrap capabilities to synchronize data from existing nodes or backups
- Configure standby instances for each pgEdge node with automatic failover managed by CloudNativePG
- Spock's delayed feedback feature and failover slots worker are configured to maintain multi-active replication across failovers and promotions
- Client certificate authentication for all managed users using cert-manager