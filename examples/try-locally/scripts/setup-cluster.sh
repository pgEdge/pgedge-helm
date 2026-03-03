#!/bin/bash
set -euo pipefail

# Sets up a Kubernetes cluster and installs CNPG operator + cert-manager.
# If an existing cluster is detected (via EXISTING_CLUSTER env var or kubectl),
# skips kind entirely and installs operators on the current cluster.

CLUSTER_NAME="${1:-pgedge-demo}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# --- Detect existing Kubernetes cluster ---
USE_EXISTING=false
if [ "${EXISTING_CLUSTER:-}" = "true" ]; then
  USE_EXISTING=true
elif command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  # No EXISTING_CLUSTER env var, but kubectl can reach a cluster
  USE_EXISTING=true
fi

# Wait for a deployment to become available, with retry on failure.
# On timeout, checks for image pull errors and offers to retry.
wait_for_deployment() {
  local namespace="$1"
  local selector="$2"
  local label="$3"
  local -a selector_args

  if [ "$selector" = "--all" ]; then
    selector_args=(--all)
  else
    selector_args=(-l "$selector")
  fi

  while true; do
    if kubectl wait --for=condition=Available deployment "${selector_args[@]}" -n "$namespace" --timeout=120s 2>/dev/null; then
      return 0
    fi

    echo ""
    echo "Timed out waiting for ${label}. Checking why..."
    echo ""

    # Check for image pull issues
    local pull_errors
    pull_errors=$(kubectl get pods -n "$namespace" -o jsonpath='{range .items[*]}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' 2>/dev/null | grep -c "ImagePull" || true)

    if [ "$pull_errors" -gt 0 ]; then
      echo "  Container images are failing to download. This is usually a"
      echo "  transient issue with the container registry (rate limits, outages)."
      echo ""
      kubectl get pods -n "$namespace" --no-headers 2>/dev/null | while read -r line; do
        echo "  $line"
      done
    else
      echo "  Pods are not ready yet:"
      echo ""
      kubectl get pods -n "$namespace" --no-headers 2>/dev/null | while read -r line; do
        echo "  $line"
      done
    fi

    echo ""
    read -rp "  Retry? [Y/n] " answer </dev/tty
    case "${answer:-y}" in
      [nN]*) echo "Aborting."; exit 1 ;;
    esac

    # Delete failed pods to force a fresh pull attempt
    if [ "$pull_errors" -gt 0 ]; then
      echo ""
      echo "  Deleting failed pods to retry image pulls..."
      kubectl delete pods -n "$namespace" --field-selector=status.phase!=Running --ignore-not-found 2>/dev/null || true
      sleep 5
    fi

    echo ""
    echo "  Waiting for ${label}..."
  done
}

echo "=== Setting up Kubernetes cluster ==="

# Check prerequisites — only require kind when creating a local cluster
if [ "$USE_EXISTING" = true ]; then
  for cmd in kubectl helm; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "Error: $cmd is not installed."
      echo "  Run the install script which will install it automatically:"
      echo "  curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/try-locally/install.sh | bash"
      exit 1
    fi
  done
else
  for cmd in kind kubectl helm; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "Error: $cmd is not installed."
      echo "  Run the install script which will install it automatically:"
      echo "  curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/try-locally/install.sh | bash"
      exit 1
    fi
  done
fi

# Set up cluster
if [ "$USE_EXISTING" = true ]; then
  echo "Detected existing Kubernetes cluster:"
  echo ""
  kubectl cluster-info 2>/dev/null | head -2
  echo ""
  echo "This will install cert-manager and CloudNativePG operator on your cluster."
  read -rp "Continue? [Y/n] " answer </dev/tty
  case "${answer:-y}" in
    [nN]*) echo "Aborted."; exit 1 ;;
  esac
  echo "existing" > "${REPO_ROOT}/.cluster-mode"
else
  # Create kind cluster if it doesn't exist
  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Kind cluster '${CLUSTER_NAME}' already exists, reusing it."
  else
    echo "Creating kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name "$CLUSTER_NAME" --wait 60s
  fi
  echo "kind" > "${REPO_ROOT}/.cluster-mode"
fi

echo ""
echo "Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
echo "Waiting for cert-manager..."
wait_for_deployment "cert-manager" "--all" "cert-manager"

echo ""
echo "Adding pgEdge Helm repo..."
helm repo add pgedge https://pgedge.github.io/charts 2>/dev/null || true
helm repo update

echo ""
echo "Installing pgEdge CloudNativePG operator..."
helm upgrade --install cnpg pgedge/cloudnative-pg --namespace cnpg-system --create-namespace
echo "Waiting for CNPG operator..."
wait_for_deployment "cnpg-system" "app.kubernetes.io/name=cloudnative-pg" "CNPG operator"

# Install pgEdge cnpg kubectl plugin if missing
if ! kubectl cnpg version &>/dev/null; then
  echo ""
  echo "Installing cnpg kubectl plugin..."
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
  curl -sSfL "https://github.com/pgEdge/pgedge-cnpg-dist/releases/download/v1.28.0/kubectl-cnpg-${OS}-${ARCH}.tar.gz" \
    | sudo tar xz -C /usr/local/bin
fi

echo ""
echo "=== Cluster is ready! ==="
echo ""
kubectl get nodes
