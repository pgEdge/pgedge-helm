# shellcheck shell=bash
# runner.sh — Reusable terminal UX framework for interactive walkthrough guides.
#
# This file is SOURCED by guide scripts (not executed directly).
# It provides ANSI color definitions, formatted output helpers, an interactive
# prompt-and-run loop, and a braille-dot spinner for long-running operations.
#
# Usage:
#   source "$(dirname "$0")/runner.sh"

# --- Colors and formatting (pgEdge brand: #449cbf teal, #eba96c orange) ---
BOLD='\033[1m'
TEAL='\033[38;5;30m'
ORANGE='\033[38;5;172m'
GREEN='\033[0;32m'
DIM='\033[2m'
RESET='\033[0m'

# --- Output helpers ---

header() {
  echo ""
  echo -e "${BOLD}${TEAL}══════════════════════════════════════════════════════════════${RESET}"
  echo -e "${BOLD}${TEAL}$1${RESET}"
  echo -e "${BOLD}${TEAL}══════════════════════════════════════════════════════════════${RESET}"
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
  echo -e "${ORANGE}\$ $1${RESET}"
}

# --- Interactive prompts ---

prompt_run() {
  local cmd="$1"
  local slow="${2:-}"
  show_cmd "$cmd"
  echo ""
  read -rp "Press Enter to run..." </dev/tty
  echo ""
  if [ -n "$slow" ]; then
    local tmpfile
    tmpfile=$(mktemp)
    start_spinner "$slow"
    eval "$cmd" > "$tmpfile" 2> >(grep -v "Unable to use a TTY" >&2)
    stop_spinner
    echo -e "${DIM}─── Output ─────────────────────────────────────────────────${RESET}"
    cat "$tmpfile"
    echo -e "${DIM}────────────────────────────────────────────────────────────${RESET}"
    rm -f "$tmpfile"
  else
    echo -e "${DIM}─── Output ─────────────────────────────────────────────────${RESET}"
    eval "$cmd" 2> >(grep -v "Unable to use a TTY" >&2)
    echo -e "${DIM}────────────────────────────────────────────────────────────${RESET}"
  fi
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
        printf "\r\033[38;5;30m%s\033[0m %s" "${chars:$i:1}" "$msg"
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
