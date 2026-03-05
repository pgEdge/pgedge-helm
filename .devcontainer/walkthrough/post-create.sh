#!/bin/bash
set -euo pipefail

echo "=== pgEdge Helm Walkthrough — Codespaces Setup ==="

ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# Install kind (not provided by devcontainer features)
echo "Installing kind..."
KIND_VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -Lo /tmp/kind "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${ARCH}"
chmod +x /tmp/kind
sudo mv /tmp/kind /usr/local/bin/kind

# Install pgEdge cnpg kubectl plugin
echo "Installing cnpg kubectl plugin..."
curl -sSfL "https://github.com/pgEdge/pgedge-cnpg-dist/releases/download/v1.28.0/kubectl-cnpg-linux-${ARCH}.tar.gz" \
  | sudo tar xz -C /usr/local/bin

# Create kind cluster (tools already on PATH so setup.sh skips tool installation)
echo ""
bash examples/walkthrough/setup.sh

echo ""
echo "Setup complete!"
echo "  Interactive Guide: cd examples/walkthrough && ./guide.sh"
echo "  Walkthrough:       Open docs/walkthrough.md (Runme extension installed)"
