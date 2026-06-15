# Changelog

All notable changes to pgEdge Helm will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v1.0.0 - 2026-05-22

### Added

- Added support for zero-downtime node addition using a Spock populate pipeline. Writes can continue uninterrupted on existing nodes while a new node syncs via logical replication, replacing the previous behavior which required stopping writes during `helm upgrade` (#52)
- Added a guided walkthrough that progressively builds an active-active PostgreSQL deployment, available as documentation, an interactive terminal guide, and a one-click GitHub Codespaces environment (#48)
- Added the ability to configure the database name, owner, and admin role via Helm values (`initdb.database`, `initdb.owner`, `pgEdge.adminUser`). Also added documentation covering all managed users, their authentication methods, and how to create additional users or disable passwords (#54)

### Changed

- Rewrote the quickstart documentation for experienced Kubernetes users with a streamlined, copy-paste-and-go guide that assumes a working cluster (#46)
- Updated default Postgres image to 18, CloudNativePG to 1.29, and cert-manager to use the latest release (#51)
- Rewrote the init-spock job from Python to Go with a declarative resource engine that inspects actual state and converges toward desired state. Includes parallel execution within dependency phases and reduces the Docker image from ~150MB to ~15MB (#50)

### Fixed

- Fixed init-spock admin connection pool to honor internalHostname for cluster-local routing, matching the existing health check behavior

## v0.2.0 - 2026-02-11

### Added

- Added security context configuration for init-spock job, allowing customization of `runAsUser`, `runAsGroup`, `fsGroup`, and `runAsNonRoot` settings (thanks @kevinpthorne)
- Added `extraResources` field to deploy additional Kubernetes resources (NetworkPolicies, PodMonitors, ConfigMaps, etc.) alongside pgEdge with full Helm template evaluation
- Added `internalHostname` configuration option for nodes, allowing in-cluster connection checks while preserving the external hostname for spock configuration (useful when clusters prevent hairpinning)
- Added automated release workflow which manages release notes, builds/pushes Docker images, GPG-signs Helm charts, and creates GitHub releases with prerelease support
- Added pgEdge Helm Repository (https://pgedge.github.io/charts) providing official Helm charts for pgEdge, CloudNativePG, and Barman Cloud plugin with updated documentation recommending this as the primary installation method

### Changed

- init-spock job image now defaults to `ghcr.io/pgedge/pgedge-helm-utils:v<chart-version>` when `initSpockImageName` is not set, eliminating manual version updates during releases
- Improved Helm validation logic so `bootstrap.mode` is only required when `initSpock` is enabled

### Fixed

- Fixed duplicate `privateKey` key in client-ca Certificate resource
## v0.1.0 - 2025-12-19

### Added

- Exposed the ability to override the container image used in the init-spock job (thanks @marlenekoh)
- Added documentation describing how to load and configure extensions which are available in the [pgEdge Enterprise Postgres Standard image](https://github.com/pgEdge/postgres-images)

### Changed

- Moved the init-spock job to utilize the [pgedge-helm-utils](https://github.com/pgEdge/pgedge-helm/pkgs/container/pgedge-helm-utils) image instead of downloading python dependencies at runtime
  - The image uses a distroless base image and includes build revision support for versioned releases
  - This enables users to install this chart in on-premise environments, or environments where security controls may have blocked the python dependency installs
- Updated documentation and tests to utilize [CloudNativePG 1.28.0](https://cloudnative-pg.io/releases/cloudnative-pg-1-28.0-released/)

## v0.0.4 - 2025-11-04

### Added

- Added a quickstart to the docs

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

