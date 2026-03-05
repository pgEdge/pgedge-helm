#!/bin/bash
set -euo pipefail

# Entrypoint for curl-pipe installation:
#   curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/walkthrough/install.sh | bash

WORK_DIR="${WALKTHROUGH_DIR:-pgedge-walkthrough}"
BRANCH="${WALKTHROUGH_BRANCH:-feature/walkthroughs}"
BASE_URL="https://raw.githubusercontent.com/pgEdge/pgedge-helm/${BRANCH}"

# --- Header ---

echo ""
echo "  pgEdge Helm Walkthrough"
echo "  ======================="
echo ""

# --- OS / architecture detection ---

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
echo "  Detected ${OS}/${ARCH}"

# --- Download walkthrough files (mirrors repo layout) ---

echo "  Downloading walkthrough files..."
mkdir -p "$WORK_DIR/examples/walkthrough/values"
mkdir -p "$WORK_DIR/docs"

FILES=(
  examples/walkthrough/guide.sh
  examples/walkthrough/runner.sh
  examples/walkthrough/setup.sh
  examples/walkthrough/values/step1-single-primary.yaml
  examples/walkthrough/values/step2-with-replicas.yaml
  examples/walkthrough/values/step3-multi-master.yaml
  docs/walkthrough.md
)

for file in "${FILES[@]}"; do
  curl -fsSL "$BASE_URL/$file" -o "$WORK_DIR/$file"
done

chmod +x "$WORK_DIR/examples/walkthrough/guide.sh" "$WORK_DIR/examples/walkthrough/setup.sh"

cd "$WORK_DIR"

# --- Run setup (tools + cluster only, no operators) ---

echo ""
bash examples/walkthrough/setup.sh

# --- Present choices ---

echo ""
echo "  Setup complete! Next, run:"
echo ""
echo "    cd $WORK_DIR"
echo ""
echo "  Then choose how to continue:"
echo ""
echo "    Interactive Guide (terminal):  cd examples/walkthrough && ./guide.sh"
echo "    Walkthrough (VS Code + Runme): code docs/walkthrough.md"
echo ""
