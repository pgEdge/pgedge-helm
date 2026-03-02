#!/bin/bash
set -euo pipefail

# Interactive CLI guide for pgEdge Kubernetes demo.
# Walks through the same progressive journey as the Killercoda scenario.

# --- Colors and formatting ---
BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
DIM='\033[2m'
RESET='\033[0m'

header() {
  echo ""
  echo -e "${BOLD}${BLUE}══════════════════════════════════════════════════════════════${RESET}"
  echo -e "${BOLD}${BLUE}  $1${RESET}"
  echo -e "${BOLD}${BLUE}══════════════════════════════════════════════════════════════${RESET}"
  echo ""
}

explain() {
  echo -e "  $1"
}

show_cmd() {
  echo ""
  echo -e "  ${YELLOW}\$ $1${RESET}"
}

prompt_run() {
  local cmd="$1"
  show_cmd "$cmd"
  echo ""
  read -rp "  Press Enter to run..." </dev/tty
  echo -e "  ${CYAN}⏳ Running...${RESET}"
  echo ""
  eval "$cmd" 2> >(grep -v "Unable to use a TTY" >&2)
  echo ""
}

prompt_continue() {
  echo ""
  read -rp "  Press Enter to continue..." </dev/tty
  echo ""
}

SPINNER_PID=""
start_spinner() {
  local msg="$1"
  local chars='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  (
    while true; do
      for (( i=0; i<${#chars}; i++ )); do
        printf "\r  \033[0;36m%s\033[0m %s" "${chars:$i:1}" "$msg"
        sleep 0.1
      done
    done
  ) &
  SPINNER_PID=$!
}

stop_spinner() {
  if [ -n "$SPINNER_PID" ]; then
    kill "$SPINNER_PID" 2>/dev/null
    wait "$SPINNER_PID" 2>/dev/null || true
    printf "\r\033[K"
    SPINNER_PID=""
  fi
}

info() {
  echo -e "  ${GREEN}$1${RESET}"
}

trap 'stop_spinner' EXIT

VALUES_DIR="$(cd "$(dirname "$0")" && pwd)/values"

# ============================================================
header "pgEdge Distributed Postgres on Kubernetes"
# ============================================================

echo "  This guide walks you through building a distributed PostgreSQL"
echo "  cluster, one step at a time:"
echo ""
echo "    1. Set up a Kubernetes cluster with the required operators"
echo "    2. Deploy a single PostgreSQL primary"
echo "    3. Add a synchronous read replica for HA"
echo "    4. Add a second node for multi-master replication"
echo "    5. Prove active-active replication works"
echo ""
echo "  Each step is a helm install or upgrade — you'll see the cluster"
echo "  evolve from a single database to a distributed system."

prompt_continue

# ============================================================
header "Step 1: Set Up Kubernetes Cluster"
# ============================================================

CLUSTER_MODE_FILE="$(cd "$(dirname "$0")" && pwd)/.cluster-mode"

# Detect if we'll be using an existing cluster (before setup-cluster.sh writes the marker)
if [ "${EXISTING_CLUSTER:-}" = "true" ] || \
   (command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1 && \
    ! command -v kind &>/dev/null); then
  explain "Your Kubernetes cluster is already running. We'll install two operators:"
  echo ""
  explain "  - ${BOLD}cert-manager${RESET}     Manages TLS certificates for secure replication"
  explain "  - ${BOLD}CloudNativePG${RESET}    Operator that manages PostgreSQL as K8s resources"
else
  explain "First we need a Kubernetes cluster with three components:"
  echo ""
  explain "  - ${BOLD}kind${RESET}             Local K8s cluster running in Docker"
  explain "  - ${BOLD}cert-manager${RESET}     Manages TLS certificates for secure replication"
  explain "  - ${BOLD}CloudNativePG${RESET}    Operator that manages PostgreSQL as K8s resources"
fi
echo ""
explain "This takes about 2 minutes. The script handles it automatically."

prompt_continue

bash "$(dirname "$0")/scripts/setup-cluster.sh"

# Read cluster mode from marker file written by setup-cluster.sh
CLUSTER_MODE="kind"
if [ -f "$CLUSTER_MODE_FILE" ]; then
  CLUSTER_MODE=$(cat "$CLUSTER_MODE_FILE")
fi

echo ""
info "Cluster is ready with all operators installed."
prompt_continue

# ============================================================
header "Step 2: Deploy a Single Primary"
# ============================================================

explain "Let's start with the simplest possible deployment: one pgEdge"
explain "node running a single PostgreSQL instance."
echo ""
explain "The values file defines just one node (n1) with 1 instance:"
echo ""
echo -e "  ${DIM}pgEdge:"
echo -e "    nodes:"
echo -e "      - name: n1"
echo -e "        hostname: pgedge-n1-rw"
echo -e "    clusterSpec:"
echo -e "      instances: 1${RESET}"

prompt_run "helm install pgedge pgedge/pgedge -f ${VALUES_DIR}/step1-single-primary.yaml"

explain "The CNPG operator is now creating a PostgreSQL pod."
echo ""
start_spinner "Waiting for pod to be ready..."
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s 2>/dev/null || true
stop_spinner
echo ""

explain "Now let's check the cluster status. This shows instance count,"
explain "replication state, and overall health:"

prompt_run "kubectl cnpg status pgedge-n1"

explain "Let's verify we can connect to the database. The pgEdge chart"
explain "creates a database called 'app' with the Spock extension:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT version();'"

info "Single primary is running — one node, one instance, no replication yet."
prompt_continue

# ============================================================
header "Step 3: Scale with Read Replicas"
# ============================================================

explain "Now we'll upgrade the deployment to add a ${BOLD}synchronous read replica${RESET}."
explain "This gives you HA — if the primary fails, the replica takes over"
explain "with zero data loss."
echo ""
explain "The change is a helm upgrade with an updated values file."
explain "Key difference from step 1:"
echo ""
echo -e "  ${DIM}instances: 1  →  instances: 2"
echo -e "  + synchronous replication with dataDurability: required${RESET}"

prompt_run "helm upgrade pgedge pgedge/pgedge -f ${VALUES_DIR}/step2-with-replicas.yaml"

explain "A second pod is spinning up as a synchronous replica..."
echo ""
start_spinner "Waiting for replica to be ready..."
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s 2>/dev/null || true
stop_spinner
echo ""

explain "Let's check the status — you should see 2 instances now,"
explain "with the replica in 'Standby (sync)' role:"

prompt_run "kubectl cnpg status pgedge-n1"

explain "We can verify replication directly in PostgreSQL."
explain "Look for sync_state = 'sync' or 'quorum':"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT client_addr, state, sync_state FROM pg_stat_replication;'"

info "HA cluster running — primary + synchronous replica, zero data loss on failover."
prompt_continue

# ============================================================
header "Step 4: Go Multi-Master"
# ============================================================

explain "This is where pgEdge shines. We'll add a ${BOLD}second pgEdge node${RESET} (n2)"
explain "with ${BOLD}Spock active-active replication${RESET}. Both nodes will accept writes,"
explain "and changes replicate bidirectionally."
echo ""
explain "The values file adds n2 to the nodes list. The chart automatically"
explain "configures Spock logical replication between n1 and n2:"
echo ""
echo -e "  ${DIM}nodes:"
echo -e "    - name: n1    # existing, keeps its replica"
echo -e "    - name: n2    # new, bootstraps from n1 via Spock${RESET}"

prompt_run "helm upgrade pgedge pgedge/pgedge -f ${VALUES_DIR}/step3-multi-master.yaml"

explain "The CNPG operator is creating a new cluster for n2, and the"
explain "pgEdge init-spock job will wire up Spock subscriptions..."
echo ""
start_spinner "Waiting for n1 pods..."
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n1 --timeout=180s 2>/dev/null || true
stop_spinner
start_spinner "Waiting for n2 pods..."
kubectl wait --for=condition=Ready pod -l cnpg.io/cluster=pgedge-n2 --timeout=180s 2>/dev/null || true
stop_spinner
echo ""

explain "Let's check both clusters:"

prompt_run "kubectl cnpg status pgedge-n1"
prompt_run "kubectl cnpg status pgedge-n2"

explain "Now let's verify Spock subscriptions are active. Each node"
explain "subscribes to the other — that's what makes it active-active:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT * FROM spock.sub_show_status();'"

info "Multi-master cluster running — both nodes accept reads and writes."
prompt_continue

# ============================================================
header "Step 5: Prove Replication"
# ============================================================

explain "Let's prove it works: write on n1, read on n2, write on n2,"
explain "read on n1. If all data shows up everywhere, active-active"
explain "replication is working."
echo ""
explain "First, create a table on n1:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c \"
CREATE TABLE cities (
  id INT PRIMARY KEY,
  name TEXT NOT NULL,
  country TEXT NOT NULL
);\""

explain "Insert some data on n1:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c \"
INSERT INTO cities (id, name, country) VALUES
  (1, 'New York', 'USA'),
  (2, 'London', 'UK'),
  (3, 'Tokyo', 'Japan');\""

explain "Now read on n2 — these rows were written on n1 but are"
explain "already replicated to n2:"

prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c 'SELECT * FROM cities;'"

explain "Now write on n2 — the other direction:"

prompt_run "kubectl cnpg psql pgedge-n2 -- -d app -c \"
INSERT INTO cities (id, name, country) VALUES
  (4, 'Sydney', 'Australia'),
  (5, 'Berlin', 'Germany');\""

explain "And read everything back on n1. All 5 rows should be here —"
explain "3 written locally and 2 replicated from n2:"

prompt_run "kubectl cnpg psql pgedge-n1 -- -d app -c 'SELECT * FROM cities ORDER BY id;'"

info "All 5 cities on both nodes — bidirectional active-active replication confirmed!"

# ============================================================
header "Done!"
# ============================================================

echo "  You've built a distributed, active-active PostgreSQL cluster"
echo "  on Kubernetes using pgEdge — starting from a single instance"
echo "  and evolving it step by step."
echo ""
echo -e "  ${BOLD}What you built:${RESET}"
echo "    1. Single Primary        one node, one instance"
echo "    2. HA with Replicas      synchronous read replica"
echo "    3. Multi-Master          Spock active-active replication"
echo "    4. Proved Replication    bidirectional writes confirmed"
echo ""
echo -e "  ${BOLD}Useful commands:${RESET}"
echo "    kubectl cnpg status pgedge-n1        # n1 cluster health"
echo "    kubectl cnpg status pgedge-n2        # n2 cluster health"
echo "    kubectl cnpg psql pgedge-n1 -- -d app  # psql shell to n1"
echo "    kubectl cnpg psql pgedge-n2 -- -d app  # psql shell to n2"
echo "    kubectl get pods -o wide             # all pods"
echo "    helm get values pgedge               # current helm values"
echo ""
echo -e "  ${BOLD}Learn more:${RESET}"
echo "    https://github.com/pgedge/pgedge-helm"
echo "    https://docs.pgedge.com"
echo "    https://www.pgedge.com"
echo ""

echo ""
if [ "${CLUSTER_MODE:-kind}" = "existing" ]; then
  read -rp "  Would you like to clean up the demo resources? [y/N] " answer </dev/tty
  case "${answer:-n}" in
    [yY]*)
      echo ""
      echo -e "  ${CYAN}Uninstalling Helm release...${RESET}"
      helm uninstall pgedge 2>/dev/null || true
      echo ""
      read -rp "  Also remove CNPG operator and cert-manager? [y/N] " answer2 </dev/tty
      case "${answer2:-n}" in
        [yY]*)
          echo -e "  ${CYAN}Removing CloudNativePG operator...${RESET}"
          helm uninstall cnpg --namespace cnpg-system 2>/dev/null || true
          echo -e "  ${CYAN}Removing cert-manager...${RESET}"
          kubectl delete -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml 2>/dev/null || true
          ;;
      esac
      rm -f "$CLUSTER_MODE_FILE"
      echo ""
      info "All cleaned up."
      ;;
    *)
      echo ""
      echo -e "  ${BOLD}To clean up later:${RESET}"
      echo "    helm uninstall pgedge"
      echo "    helm uninstall cnpg --namespace cnpg-system"
      echo "    kubectl delete -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml"
      ;;
  esac
else
  read -rp "  Would you like to clean up the demo environment? [y/N] " answer </dev/tty
  case "${answer:-n}" in
    [yY]*)
      echo ""
      echo -e "  ${CYAN}Uninstalling Helm release...${RESET}"
      helm uninstall pgedge 2>/dev/null || true
      echo -e "  ${CYAN}Deleting kind cluster...${RESET}"
      kind delete cluster --name pgedge-demo 2>/dev/null || true
      rm -f "$CLUSTER_MODE_FILE"
      echo ""
      info "All cleaned up."
      ;;
    *)
      echo ""
      echo -e "  ${BOLD}To clean up later:${RESET}"
      echo "    helm uninstall pgedge"
      echo "    kind delete cluster --name pgedge-demo"
      ;;
  esac
fi
echo ""
