# Running Tests

The test suite includes unit tests and integration tests. Integration tests run against a real Kubernetes cluster using [kind](https://kind.sigs.k8s.io).

In CI, these tests are run automatically on every push and pull request via the [GitHub Actions workflow](../.github/workflows/test.yaml). The workflow runs unit tests, chart-testing (`ct`), and integration tests in parallel jobs against a kind cluster.

## Prerequisites

### Mac

```shell
# 1. Install Docker Desktop
# https://docs.docker.com/desktop/mac/install/

# 2. Install tools
brew install go kind helm kubectl
# Or from the repo root:
# make setup

# 3. Install gotestsum
make -C test install-gotestsum
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
```

### Linux (Rocky 9)

Install Docker:

```shell
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
sudo dnf install -y docker-ce docker-ce-cli containerd.io
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
newgrp docker
```

Install tools:

```shell
# make + git
sudo dnf install -y make git

# Go
curl -Lo /tmp/go.tar.gz https://go.dev/dl/go1.25.9.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
echo 'export PATH=$PATH:/usr/local/bin:/usr/local/go/bin:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# kubectl
KUBECTL_VERSION=$(curl -Ls https://dl.k8s.io/release/stable.txt)
curl -Lo /tmp/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod +x /tmp/kubectl && sudo mv /tmp/kubectl /usr/local/bin/kubectl

# kind
KIND_VERSION=$(curl -fsSL https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -Lo /tmp/kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64
chmod +x /tmp/kind && sudo mv /tmp/kind /usr/local/bin/kind

# Helm
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
sudo ln -sf /usr/local/bin/helm /usr/bin/helm

# gotestsum
make -C test install-gotestsum
sudo ln -sf $(go env GOPATH)/bin/gotestsum /usr/local/bin/gotestsum
```

## Running Tests

### All tests (unit + integration)

```shell
make -C test test-all TIMEOUT=120m
```

This creates a kind cluster, runs all tests, then tears the cluster down automatically.

### Unit tests only

```shell
make -C test test-unit
```

Unit tests do not require a cluster and run directly against the local code.

### Integration test groups

```shell
make -C test test-install   TIMEOUT=120m  # install tests
make -C test test-nodes     TIMEOUT=120m  # node add/remove tests
make -C test test-recovery  TIMEOUT=120m  # recovery tests
make -C test test-reset     TIMEOUT=120m  # Spock reset tests
make -C test test-failover  TIMEOUT=120m  # failover tests
```

### Single test

```shell
make -C test test-run RUN=TestNodesAddNodeZeroDowntime TIMEOUT=120m
```

### Multiple specific tests

```shell
make -C test test-run RUN="TestNodesRemoveNode|TestResetSpock" TIMEOUT=120m
```

## Chart Testing (ct)

[chart-testing](https://github.com/helm/chart-testing) is used for basic chart lint and install/upgrade testing. These targets require a kind cluster.

### Lint only (no cluster required)

```shell
make -C test test-ct-lint
```

### Chart install test

```shell
make -C test test-ct
```

This creates a kind cluster, runs `ct install` against the chart, then tears the cluster down.
