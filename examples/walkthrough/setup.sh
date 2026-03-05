#!/bin/bash
set -euo pipefail

# setup.sh — Prerequisites: tools and Kubernetes cluster.
#
# Handles two concerns:
#   1. Tool installation (local machine only) — installs to ./bin/, no sudo
#   2. Kubernetes cluster (kind or detect existing)
#
# Does NOT install operators (cert-manager, CNPG) or Helm repos — those are
# part of the guided walkthrough steps (guide.sh / walkthrough.md).
#
# Called by install.sh (curl-pipe) and devcontainer post-create.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"
CLUSTER_NAME="${CLUSTER_NAME:-pgedge-demo}"

# --- OS / Architecture detection ---

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# ═══════════════════════════════════════════════════════════════════════════════
# Part 1 — Tool installation (local machine only)
# ═══════════════════════════════════════════════════════════════════════════════

# Check which tools are already available on PATH.
NEED_INSTALL=false
MISSING=()

for cmd in kind kubectl helm; do
  if ! command -v "$cmd" &>/dev/null; then
    MISSING+=("$cmd")
    NEED_INSTALL=true
  fi
done

# cnpg plugin check (separate — it's a kubectl sub-command)
if ! kubectl cnpg version &>/dev/null 2>&1; then
  MISSING+=("kubectl-cnpg")
  NEED_INSTALL=true
fi

if [ "$NEED_INSTALL" = true ]; then
  echo "The following tools are missing and will be installed to ${BIN_DIR}:"
  echo "  ${MISSING[*]}"
  echo ""
  echo "No sudo is required — everything stays inside this directory."
  echo ""
  read -rp "Continue? [Y/n] " answer </dev/tty
  case "${answer:-y}" in
    [nN]*) echo "Aborted."; exit 1 ;;
  esac
  echo ""

  mkdir -p "$BIN_DIR"
  export PATH="$BIN_DIR:$PATH"

  for cmd in "${MISSING[@]}"; do
    case "$cmd" in
      kind)
        echo "Installing kind..."
        version=$(curl -fsSL https://api.github.com/repos/kubernetes-sigs/kind/releases/latest \
          | grep '"tag_name"' | cut -d'"' -f4)
        curl -fsSLo "$BIN_DIR/kind" "https://kind.sigs.k8s.io/dl/${version}/kind-${OS}-${ARCH}"
        chmod +x "$BIN_DIR/kind"
        echo "  ✓ kind ${version}"
        ;;
      kubectl)
        echo "Installing kubectl..."
        version=$(curl -fsSL https://dl.k8s.io/release/stable.txt)
        curl -fsSLo "$BIN_DIR/kubectl" "https://dl.k8s.io/release/${version}/bin/${OS}/${ARCH}/kubectl"
        chmod +x "$BIN_DIR/kubectl"
        echo "  ✓ kubectl ${version}"
        ;;
      helm)
        echo "Installing Helm..."
        curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
          | HELM_INSTALL_DIR="$BIN_DIR" USE_SUDO=false bash
        echo "  ✓ Helm $(helm version --short)"
        ;;
      kubectl-cnpg)
        echo "Installing cnpg kubectl plugin..."
        curl -fsSL "https://github.com/pgEdge/pgedge-cnpg-dist/releases/download/v1.28.0/kubectl-cnpg-${OS}-${ARCH}.tar.gz" \
          | tar xz -C "$BIN_DIR"
        echo "  ✓ kubectl-cnpg"
        ;;
    esac
  done

  echo ""
else
  echo "All required tools are already on PATH."
  echo ""
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Part 2 — Kubernetes cluster
# ═══════════════════════════════════════════════════════════════════════════════

# --- Detect existing Kubernetes cluster or create kind cluster ---

create_kind_cluster() {
  # Only require Docker when creating a local kind cluster
  if ! command -v docker &>/dev/null; then
    echo "Error: Docker is not installed and no existing Kubernetes cluster was detected."
    echo "  Either install Docker: https://docs.docker.com/get-docker/"
    echo "  Or configure kubectl to connect to an existing cluster."
    exit 1
  fi

  if ! docker info &>/dev/null; then
    echo "Error: Docker is installed but not running."
    echo "  Please start Docker and try again."
    exit 1
  fi

  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Kind cluster '${CLUSTER_NAME}' already exists, reusing it."
  else
    echo "Creating kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name "$CLUSTER_NAME" --wait 60s
  fi
  echo "kind" > "$SCRIPT_DIR/.cluster-mode"
}

echo "Kubernetes Cluster"
echo "──────────────────"
echo ""

if command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  CONTEXT=$(kubectl config current-context 2>/dev/null)
  SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null)
  echo "Detected existing cluster:"
  echo "  Context:  $CONTEXT"
  echo "  Server:   $SERVER"
  echo ""
  echo "The walkthrough will install cert-manager and CloudNativePG on this cluster."
  echo ""
  echo "  1) Use this cluster"
  echo "  2) Create a new kind cluster instead"
  echo ""
  read -rp "Choose [1/2]: " answer </dev/tty
  case "$answer" in
    2)
      echo ""
      create_kind_cluster
      ;;
    *)
      echo "existing" > "$SCRIPT_DIR/.cluster-mode"
      ;;
  esac
else
  create_kind_cluster
fi

echo ""
echo "Cluster is ready!"
echo ""
kubectl get nodes
