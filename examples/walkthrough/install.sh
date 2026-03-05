#!/bin/bash
set -euo pipefail

# Entrypoint for curl-pipe installation:
#   curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/walkthrough/install.sh | bash

WORK_DIR="${WALKTHROUGH_DIR:-pgedge-walkthrough}"
BRANCH="${WALKTHROUGH_BRANCH:-main}"
BASE_URL="https://raw.githubusercontent.com/pgEdge/pgedge-helm/${BRANCH}"

# --- Header ---

echo ""
echo "  pgEdge Helm Walkthrough"
echo "  ======================="
echo ""

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

export PATH="$(pwd)/examples/walkthrough/bin:$PATH"
echo ""
bash examples/walkthrough/setup.sh

# --- Choose how to continue ---

echo ""
echo "  Setup complete! How would you like to continue?"
echo ""
echo "    1) Interactive Guide — step-by-step in this terminal"
echo "    2) Exit — I'll open the walkthrough in my editor"
echo ""
read -rp "  Choose [1/2]: " choice </dev/tty

case "$choice" in
  2)
    echo ""
    echo "  Open this file in your editor and run the commands in this terminal:"
    echo "    $(pwd)/docs/walkthrough.md"
    echo ""
    ;;
  *)
    echo ""
    bash examples/walkthrough/guide.sh
    ;;
esac
