#!/bin/bash
set -euo pipefail

# Bootstrap script for curl-pipe installation:
#   curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/try-locally/install.sh | bash

REPO="https://github.com/pgEdge/pgedge-helm.git"
DIR="pgedge-helm"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Detect existing Kubernetes cluster — if kubectl can reach one, Docker/kind aren't needed
EXISTING_CLUSTER=false
if command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  EXISTING_CLUSTER=true
  echo "Detected existing Kubernetes cluster — Docker and kind are not required."
  echo ""
fi

# Hard prerequisites
if ! command -v git &>/dev/null; then
  echo "Error: git is not installed."
  echo "  Install: https://git-scm.com"
  exit 1
fi

# Auto-install missing Kubernetes tooling
install_kind() {
  echo "Installing kind..."
  local version
  version=$(curl -fsSL https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
  curl -fsSLo /tmp/kind "https://kind.sigs.k8s.io/dl/${version}/kind-${OS}-${ARCH}"
  chmod +x /tmp/kind
  sudo mv /tmp/kind /usr/local/bin/kind
  echo "  Installed kind ${version}"
}

install_kubectl() {
  echo "Installing kubectl..."
  local version
  version=$(curl -fsSL https://dl.k8s.io/release/stable.txt)
  curl -fsSLo /tmp/kubectl "https://dl.k8s.io/release/${version}/bin/${OS}/${ARCH}/kubectl"
  chmod +x /tmp/kubectl
  sudo mv /tmp/kubectl /usr/local/bin/kubectl
  echo "  Installed kubectl ${version}"
}

install_helm() {
  echo "Installing Helm..."
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  echo "  Installed Helm $(helm version --short)"
}

# Only auto-install kind when no existing cluster
TOOLS=(kubectl helm)
if [ "$EXISTING_CLUSTER" = false ]; then
  TOOLS=(kind kubectl helm)
fi

MISSING=()
for cmd in "${TOOLS[@]}"; do
  if ! command -v "$cmd" &>/dev/null; then
    MISSING+=("$cmd")
  fi
done

if [ ${#MISSING[@]} -gt 0 ]; then
  echo "The following tools are missing and will be installed: ${MISSING[*]}"
  read -rp "Continue? [Y/n] " answer </dev/tty
  case "${answer:-y}" in
    [nN]*) echo "Aborted."; exit 1 ;;
  esac
  echo ""
  for cmd in "${MISSING[@]}"; do
    "install_${cmd}"
  done
  echo ""
fi

# Re-detect existing cluster after possible tool installation
if [ "$EXISTING_CLUSTER" = false ] && command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  EXISTING_CLUSTER=true
  echo "Detected existing Kubernetes cluster — Docker and kind are not required."
  echo ""
fi

# Only require Docker if we still need to create a local kind cluster
if [ "$EXISTING_CLUSTER" = false ]; then
  if ! command -v docker &>/dev/null; then
    echo "Error: Docker is not installed and no existing Kubernetes cluster was detected."
    echo "  Either install Docker: https://docs.docker.com/get-docker/"
    echo "  Or configure kubectl to connect to an existing cluster."
    exit 1
  fi

  # Check Docker is actually running
  if ! docker info &>/dev/null; then
    echo "Error: Docker is installed but not running."
    echo "  Please start Docker and try again."
    exit 1
  fi
fi

# Clone or update
if [ -d "$DIR" ]; then
  echo "Directory '$DIR' already exists, pulling latest..."
  git -C "$DIR" pull --ff-only
else
  git clone "$REPO"
fi

cd "$DIR/examples/try-locally"
export EXISTING_CLUSTER
exec ./guide.sh
