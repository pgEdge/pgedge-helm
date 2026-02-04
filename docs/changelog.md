# Changelog

## v0.1.0 - 2025-12-19

### Added

- Exposed the ability to override the container image used in the init-spock job (thanks @marlenekoh)
- Added documentation describing how to load and configure extensions which are available in the [pgEdge Enterprise Postgres Standard image](https://github.com/pgEdge/postgres-images)

### Changed

- Moved the init-spock job to utilize the [pgedge-helm-utils](https://github.com/pgEdge/pgedge-helm/pkgs/container/pgedge-helm-utils) image instead of downloading python dependencies at runtime
  - The image uses a distroless base image and includes build revision support for versioned releases
  - This enables users to install this chart in on-premises environments, or environments where security controls may have blocked the python dependency installs
- Updated documentation and tests to utilize [CloudNativePG 1.28.0](https://cloudnative-pg.io/releases/cloudnative-pg-1-28.0-released/)

## v0.0.4 - 2025-11-04

### Added

 - Added a [quickstart](quickstart.md) to the docs

### Changed

- Updated CloudNativePG in docs / tests to use version [1.27.1](https://cloudnative-pg.io/releases/cloudnative-pg-1-27.1-released/)

## v0.0.3 - 2025-10-23

### Changed

This release moves pgedge-helm to leverage [CloudNativePG](https://cloudnative-pg.io/) to manage Postgres, providing flexible options for single-region and multi-region deployments.

### Added

- Postgres 16, 17, and 18 via [pgEdge Enterprise Postgres Images](https://github.com/pgEdge/postgres-images)
- Flexible deployment options for both single-region and multi-region deployments
  - Deploy pgEdge Enterprise Postgres in a single region with optional standby replicas
  - Deploy pgEdge Distributed Postgres across multiple regions with Spock active-active replication
- Configuring Spock replication configuration across all nodes during helm install and upgrade processes
- Best practice configuration defaults for deploying pgEdge Distributed Postgres in Kubernetes
- Extending / overriding configuration for CloudNativePG across all nodes, or on specific nodes
- Configuring standby instances with automatic failover, leveraging Spock's delayed feedback and failover slots worker to maintain active-active replication across failovers and promotions
- Adding pgEdge nodes using Spock or CloudNativePG's bootstrap capabilities to synchronize data from existing nodes or backups
- Performing Postgres major and minor version upgrades
- Client certificate authentication for managed users, including the pgedge replication user
- Configuration options to support deployments across multiple Kubernetes clusters
- Updated docs site accessible via `make docs`

## v0.0.2 - 2025-04-14

### Added

- Add support for AutoDDL
- Add example values file for AKS
- Add example ACE pod template

### Changed

- Utilize latest pgedge/pgedge images
- Set imagePullPolicy to Always when using latest images for easier testing

## v0.0.1 - 2025-04-14

### Added

- Initial release

