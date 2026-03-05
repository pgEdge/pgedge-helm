# runner.sh — Reusable terminal UX framework for interactive walkthrough guides.
#
# This file is SOURCED by guide scripts (not executed directly).
# It provides ANSI color definitions, formatted output helpers, an interactive
# prompt-and-run loop, and a braille-dot spinner for long-running operations.
#
# Usage:
#   source "$(dirname "$0")/runner.sh"

# --- Colors and formatting ---
BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
DIM='\033[2m'
RESET='\033[0m'

# --- Output helpers ---

header() {
  echo ""
  echo -e "${BOLD}${BLUE}══════════════════════════════════════════════════════════════${RESET}"
  echo -e "${BOLD}${BLUE}$1${RESET}"
  echo -e "${BOLD}${BLUE}══════════════════════════════════════════════════════════════${RESET}"
  echo ""
}

explain() {
  echo -e "$1"
}

info() {
  echo -e "${GREEN}$1${RESET}"
}

show_cmd() {
  echo ""
  echo -e "${YELLOW}\$ $1${RESET}"
}

# --- Interactive prompts ---

prompt_run() {
  local cmd="$1"
  show_cmd "$cmd"
  echo ""
  read -rp "Press Enter to run..." </dev/tty
  echo -e "${CYAN}⏳ Running...${RESET}"
  echo ""
  eval "$cmd" 2> >(grep -v "Unable to use a TTY" >&2)
  echo ""
}

prompt_continue() {
  echo ""
  read -rp "Press Enter to continue..." </dev/tty
  echo ""
}

# --- Spinner ---

SPINNER_PID=""

start_spinner() {
  local msg="$1"
  local chars='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  (
    while true; do
      for (( i=0; i<${#chars}; i++ )); do
        printf "\r\033[0;36m%s\033[0m %s" "${chars:$i:1}" "$msg"
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
