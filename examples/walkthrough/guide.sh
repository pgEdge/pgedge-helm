#!/bin/bash
set -euo pipefail

# Interactive CLI guide for pgEdge Kubernetes walkthrough.
# Walks through the same progressive journey as docs/walkthrough.md.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VALUES_DIR="$SCRIPT_DIR/values"
CLUSTER_NAME="${CLUSTER_NAME:-pgedge-demo}"

# Add local bin to PATH (tools installed by setup.sh)
export PATH="$SCRIPT_DIR/bin:$PATH"

# Source terminal framework
source "$SCRIPT_DIR/runner.sh"
trap 'stop_spinner' EXIT

# === Intro ===

header "pgEdge Enterprise Postgres on Kubernetes"

echo "This guide walks you through building an active-active"
echo "PostgreSQL deployment, one step at a time:"
echo ""
echo "  1. Set Up Kubernetes          cert-manager + CloudNativePG operators"
echo "  2. Deploy a Single Primary    one pgEdge node with Postgres + Spock"
echo "  3. Add Standby Instances      synchronous HA with zero data loss"
echo "  4. Add a Second Node          active-active replication via Spock"
echo "  5. Verify Replication         write on one node, read on the other"
echo ""
echo "Each step is a helm install or upgrade — the deployment"
echo "evolves from a single instance to an active-active deployment."

prompt_continue

# === Step 1: Set Up Kubernetes ===

header "Step 1: Set Up Kubernetes"

CLUSTER_MODE_FILE="$SCRIPT_DIR/.cluster-mode"

explain "Before deploying pgEdge Helm, a Kubernetes cluster and two operators"
explain "are required:"
echo ""
explain "  - ${BOLD}cert-manager${RESET}     Handles TLS certificates for secure communication"
explain "  - ${BOLD}CloudNativePG${RESET}    Manages PostgreSQL as native Kubernetes resources"

# --- Tools and cluster ---

if command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "unknown")
  echo ""
  info "Kubernetes cluster detected (context: $CURRENT_CONTEXT)."
  echo ""
  read -rp "Use this cluster for the walkthrough? [Y/n] " answer </dev/tty
  case "${answer:-y}" in
    [nN]*)
      echo ""
      echo "To use a different cluster, switch context and re-run the guide:"
      echo "  kubectl config use-context <your-context>"
      exit 0
      ;;
  esac
  if [ ! -f "$CLUSTER_MODE_FILE" ]; then
    echo "existing" > "$CLUSTER_MODE_FILE"
  fi
else
  echo ""
  explain "The setup script installs any missing tools and creates a local"
  explain "kind cluster."
  prompt_continue
  bash "$SCRIPT_DIR/setup.sh"
fi

# Read cluster mode from marker file written by setup.sh
CLUSTER_MODE="kind"
if [ -f "$CLUSTER_MODE_FILE" ]; then
  CLUSTER_MODE=$(cat "$CLUSTER_MODE_FILE")
fi

# --- cert-manager ---

echo ""
if kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=5s &>/dev/null 2>&1; then
  info "cert-manager is already installed."
else
  explain "Installing cert-manager — handles TLS certificates so database"
  explain "nodes communicate securely:"

  prompt_run "kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml" "Installing cert-manager..."

  start_spinner "Waiting for cert-manager..."
  if ! kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s &>/dev/null; then
    stop_spinner
    echo ""
    echo "Timed out waiting for cert-manager."
    exit 1
  fi
  stop_spinner
fi

# --- Helm repo ---

echo ""
explain "Adding the pgEdge Helm repository:"

prompt_run "helm repo add pgedge https://pgedge.github.io/charts --force-update && helm repo update" "Adding Helm repo..."

# --- CNPG operator ---

echo ""
if kubectl wait --for=condition=Available deployment -l app.kubernetes.io/name=cloudnative-pg -n cnpg-system --timeout=5s &>/dev/null 2>&1; then
  info "CloudNativePG operator is already installed."
else
  explain "Installing the pgEdge CloudNativePG operator — manages PostgreSQL"
  explain "clusters as native Kubernetes resources:"

  prompt_run "helm upgrade --install cnpg pgedge/cloudnative-pg --namespace cnpg-system --create-namespace" "Installing CNPG operator..."

  start_spinner "Waiting for CNPG operator..."
  if ! kubectl wait --for=condition=Available deployment -l app.kubernetes.io/name=cloudnative-pg -n cnpg-system --timeout=120s &>/dev/null; then
    stop_spinner
    echo ""
    echo "Timed out waiting for CNPG operator."
    exit 1
  fi
  stop_spinner
fi

echo ""
info "Kubernetes is ready with all operators installed."
prompt_continue

# === Step 2: Deploy a Single Primary Instance ===

header "Step 2: Deploy a Single Primary Instance"

explain "This step deploys the simplest possible configuration: one pgEdge"
explain "node running a single PostgreSQL instance."
echo ""
explain "The values file defines one node (n1) with a single instance:"
echo ""
echo -e "${DIM}pgEdge:"
echo -e "  appName: pgedge"
echo -e "  nodes:"
echo -e "    - name: n1"
echo -e "      hostname: pgedge-n1-rw"
echo -e "      clusterSpec:"
echo -e "        instances: 1"
echo -e "  clusterSpec:"
echo -e "    storage:"
echo -e "      size: 1Gi${RESET}"

prompt_run "helm install pgedge pgedge/pgedge -f \"$VALUES_DIR/step1-single-primary.yaml\"" "Deploying single primary..."

start_spinner "Waiting for instance to be ready..."
if ! kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s &>/dev/null; then
  stop_spinner
  echo ""
  echo "Timed out waiting for pgedge-n1."
  exit 1
fi
stop_spinner
echo ""

explain "Checking the cluster status — instance count, replication state,"
explain "and overall health:"

prompt_run "kubectl cnpg status pgedge-n1"

explain "The pgEdge Helm chart creates a database called 'app' with the"
explain "Spock extension pre-installed. Verifying the database is accessible:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT version();'"

explain "Loading some data that will carry forward through the walkthrough:"

# Clean up any leftover data from a previous run
kubectl cnpg psql pgedge-n1 -- -d app -c "DROP TABLE IF EXISTS cities;" </dev/null >/dev/null 2>&1 || true

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c \"
CREATE TABLE cities (
  id INT PRIMARY KEY,
  name TEXT NOT NULL,
  country TEXT NOT NULL
);

INSERT INTO cities (id, name, country) VALUES
  (1, 'New York', 'USA'),
  (2, 'London', 'UK'),
  (3, 'Tokyo', 'Japan');\""

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT * FROM cities;'"

info "Single primary instance is running with data."
prompt_continue

# === Step 3: Add Standby Instances ===

header "Step 3: Add Standby Instances"

explain "This step upgrades the deployment to add a ${BOLD}synchronous standby${RESET}"
explain "instance. Standby instances provide high availability — if the"
explain "primary fails, a standby takes over with zero data loss."
echo ""
explain "The updated values file:"
echo ""
echo -e "${DIM}pgEdge:"
echo -e "  appName: pgedge"
echo -e "  nodes:"
echo -e "    - name: n1"
echo -e "      hostname: pgedge-n1-rw"
echo -e "      clusterSpec:"
echo -e "        instances: 2"
echo -e "        postgresql:"
echo -e "          synchronous:"
echo -e "            method: any"
echo -e "            number: 1"
echo -e "            dataDurability: required"
echo -e "  clusterSpec:"
echo -e "    storage:"
echo -e "      size: 1Gi${RESET}"

prompt_run "helm upgrade pgedge pgedge/pgedge -f \"$VALUES_DIR/step2-with-replicas.yaml\"" "Adding standby instance..."

start_spinner "Waiting for both instances to be ready..."
if ! kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s &>/dev/null; then
  stop_spinner
  echo ""
  echo "Timed out waiting for pgedge-n1 instances."
  exit 1
fi
stop_spinner
echo ""

explain "Two instances should now be visible — one primary and one standby"
explain "with the (sync) role:"

prompt_run "kubectl cnpg status pgedge-n1"

explain "Verifying replication from the primary's perspective."
explain "Look for sync_state = 'sync' or 'quorum':"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT client_addr, state, sync_state FROM pg_stat_replication;'"

info "Standby instance running — synchronous replication with zero data loss on failover."
prompt_continue

# === Step 4: Add a Second Node ===

header "Step 4: Add a Second Node"

explain "This is where pgEdge shines. This step adds a ${BOLD}second pgEdge node${RESET}"
explain "(n2) with ${BOLD}Spock active-active replication${RESET}. Both nodes accept writes,"
explain "and changes replicate bidirectionally."
echo ""
explain "Unlike the standby instance in step 3 (which exists for failover),"
explain "both n1 and n2 independently accept reads and writes."
echo ""
explain "The chart clones data from n1 and sets up Spock logical replication"
explain "between the two nodes:"
echo ""
echo -e "${DIM}pgEdge:"
echo -e "  appName: pgedge"
echo -e "  nodes:"
echo -e "    - name: n1"
echo -e "      hostname: pgedge-n1-rw"
echo -e "      clusterSpec:"
echo -e "        instances: 2"
echo -e "        postgresql:"
echo -e "          synchronous:"
echo -e "            method: any"
echo -e "            number: 1"
echo -e "            dataDurability: required"
echo -e "    - name: n2"
echo -e "      hostname: pgedge-n2-rw"
echo -e "      clusterSpec:"
echo -e "        instances: 1"
echo -e "      bootstrap:"
echo -e "        mode: spock"
echo -e "        sourceNode: n1"
echo -e "  clusterSpec:"
echo -e "    storage:"
echo -e "      size: 1Gi${RESET}"

prompt_run "helm upgrade pgedge pgedge/pgedge -f \"$VALUES_DIR/step3-multi-master.yaml\"" "Adding node n2..."

explain "The CNPG operator is creating a new cluster for n2, and the"
explain "pgEdge init-spock job wires up Spock subscriptions..."
echo ""
start_spinner "Waiting for n1..."
if ! kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s &>/dev/null; then
  stop_spinner
  echo ""
  echo "Timed out waiting for pgedge-n1."
  exit 1
fi
stop_spinner
start_spinner "Waiting for n2..."
if ! kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n2 --timeout=180s &>/dev/null; then
  stop_spinner
  echo ""
  echo "Timed out waiting for pgedge-n2."
  exit 1
fi
stop_spinner
echo ""

explain "Checking both nodes:"

prompt_run "kubectl cnpg status pgedge-n1"
prompt_run "kubectl cnpg status pgedge-n2"

explain "Verifying Spock subscriptions are active. Each node subscribes"
explain "to the other — that is what makes it active-active:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT * FROM spock.sub_show_status();'"
prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c 'SELECT * FROM spock.sub_show_status();'"

explain "The cities table was created on n1 in step 2. The bootstrap"
explain "process cloned everything to n2. Querying n2 to confirm:"

prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c 'SELECT * FROM cities;'"

info "Both nodes running with data — active-active replication is live."
prompt_continue

# === Step 5: Verify Replication ===

header "Step 5: Verify Replication"

explain "The data on n2 was bootstrapped from n1, but active-active means"
explain "both nodes can accept writes going forward. This step verifies"
explain "bidirectional replication."
echo ""
explain "Inserting two new cities on n2:"

prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c \"
INSERT INTO cities (id, name, country) VALUES
  (4, 'Sydney', 'Australia'),
  (5, 'Berlin', 'Germany');\""

explain "Reading back on n1 — all 5 rows should be present:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT * FROM cities ORDER BY id;'"

explain "Now the other direction — inserting a city on n1:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c \"
INSERT INTO cities (id, name, country) VALUES
  (6, 'Paris', 'France');\""

explain "Reading on n2 — all 6 cities should appear:"

prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c 'SELECT * FROM cities ORDER BY id;'"

info "All 6 cities on both nodes — bidirectional active-active replication confirmed."

# === Done + Cleanup ===

header "Done!"

echo "You've built an active-active PostgreSQL deployment on"
echo "Kubernetes using pgEdge Helm — starting from a single instance"
echo "and evolving it step by step."
echo ""
echo -e "${BOLD}What you built:${RESET}"
echo "  1. Set Up Kubernetes          cert-manager + CloudNativePG operators"
echo "  2. Single Primary Instance    one pgEdge node with Postgres + Spock"
echo "  3. Standby Instances          synchronous HA with zero data loss"
echo "  4. Second Node                active-active replication via Spock"
echo "  5. Verified Replication       bidirectional writes confirmed"
echo ""
echo -e "${BOLD}Useful commands:${RESET}"
echo "  kubectl cnpg status pgedge-n1        # n1 health"
echo "  kubectl cnpg status pgedge-n2        # n2 health"
echo "  kubectl cnpg psql pgedge-n1 -- -d app  # psql shell to n1"
echo "  kubectl cnpg psql pgedge-n2 -- -d app  # psql shell to n2"
echo "  kubectl get pods -o wide             # all pods"
echo "  helm get values pgedge               # current helm values"
echo ""
echo -e "${BOLD}Learn more:${RESET}"
echo "  https://github.com/pgedge/pgedge-helm"
echo "  https://docs.pgedge.com"
echo ""

echo ""
if [ "${CLUSTER_MODE:-kind}" = "existing" ]; then
  read -rp "Would you like to clean up the demo resources? [y/N] " answer </dev/tty
  case "${answer:-n}" in
    [yY]*)
      echo ""
      echo -e "${TEAL}Uninstalling Helm release...${RESET}"
      helm uninstall pgedge 2>/dev/null || true
      echo ""
      read -rp "Also remove CNPG operator and cert-manager? [y/N] " answer2 </dev/tty
      case "${answer2:-n}" in
        [yY]*)
          echo -e "${TEAL}Removing CloudNativePG operator...${RESET}"
          helm uninstall cnpg --namespace cnpg-system 2>/dev/null || true
          echo -e "${TEAL}Removing cert-manager...${RESET}"
          kubectl delete -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml 2>/dev/null || true
          ;;
      esac
      rm -f "$CLUSTER_MODE_FILE"
      echo ""
      info "All cleaned up."
      ;;
    *)
      echo ""
      echo -e "${BOLD}To clean up later:${RESET}"
      echo "  helm uninstall pgedge"
      echo "  helm uninstall cnpg --namespace cnpg-system"
      echo "  kubectl delete -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml"
      ;;
  esac
else
  read -rp "Would you like to clean up the demo environment? [y/N] " answer </dev/tty
  case "${answer:-n}" in
    [yY]*)
      echo ""
      echo -e "${TEAL}Uninstalling Helm release...${RESET}"
      helm uninstall pgedge 2>/dev/null || true
      echo -e "${TEAL}Deleting kind cluster...${RESET}"
      kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
      rm -f "$CLUSTER_MODE_FILE"
      echo ""
      info "All cleaned up."
      ;;
    *)
      echo ""
      echo -e "${BOLD}To clean up later:${RESET}"
      echo "  helm uninstall pgedge"
      echo "  kind delete cluster --name $CLUSTER_NAME"
      ;;
  esac
fi
echo ""
