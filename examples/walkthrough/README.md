# Guided Walkthrough — Developer Reference

This directory contains an interactive walkthrough that progressively builds an active-active PostgreSQL deployment using the pgEdge Helm chart. It serves as both a learning tool for users and a showcase of the chart's capabilities.

> The end-user walkthrough is at [docs/walkthrough.md](../../docs/walkthrough.md).
> This README covers how the walkthrough is structured and how to run it.

## How Users Reach the Walkthrough

The walkthrough is self-contained — it does not clone this repository or depend on git. There are two primary entrypoints:

1. `curl ... | bash` — a one-liner that runs `install.sh` remotely. It downloads the walkthrough files, installs tools locally, creates a kind cluster, and launches the interactive guide. Requires only Docker.

2. GitHub Codespaces — opens a pre-configured environment with everything installed. The devcontainer runs `setup.sh` during creation and opens `docs/walkthrough.md` with runnable code blocks via the Runme extension. Users can also run the interactive guide (`guide.sh`) from the terminal.

The walkthrough commands are also available in [docs/walkthrough.md](../../docs/walkthrough.md), which is published on [docs.pgedge.com](https://docs.pgedge.com).

If a user runs the `curl ... | bash` install inside Codespaces, `install.sh` detects the `$CODESPACES` environment variable and exits as a no-op — the devcontainer has already handled setup.

Both paths walk through the same progression:

1. Set Up Kubernetes — install cert-manager + CloudNativePG operators
2. Deploy a Single Primary — one pgEdge node, one Postgres instance
3. Add Standby Instances — synchronous HA with zero data loss
4. Add a Second Node — Spock active-active replication
5. Verify Replication — bidirectional writes confirmed

Each step maps to a `helm install` or `helm upgrade` using progressively more complex values files.

## File Overview

```
examples/walkthrough/
├── install.sh              # Curl-pipe entry point (downloads files, runs setup)
├── guide.sh                # Interactive guide (5 steps)
├── setup.sh                # Tool installation + cluster detection/creation
├── runner.sh               # Terminal UX framework (sourced, not executed)
├── .cluster-mode           # Marker file: "kind" or "existing" (generated)
└── values/
    ├── step1-single-primary.yaml   # 1 node, 1 instance
    ├── step2-with-replicas.yaml    # 1 node, 2 instances (sync standby)
    └── step3-multi-master.yaml     # 2 nodes, Spock active-active
```

### install.sh

Entry point for `curl ... | bash`. Downloads individual files from GitHub (no git clone) into a self-contained `pgedge-walkthrough/` directory that mirrors the repo layout. Runs `setup.sh` for tools and cluster setup, then prompts the user to choose between the interactive guide (`guide.sh`) or the markdown walkthrough (`docs/walkthrough.md`).

Environment variables:
- `WALKTHROUGH_DIR` — override output directory (default: `pgedge-walkthrough`)
- `WALKTHROUGH_BRANCH` — override GitHub branch (default: `main`)

In Codespaces (`$CODESPACES` is set), it exits early as a no-op since the devcontainer has already handled setup.

### setup.sh

Handles two concerns, separated clearly in the script:

Part 1 — Tool installation (local machine only, no sudo):
- Checks PATH for `kind`, `kubectl`, `helm`, `kubectl-cnpg`
- Installs missing tools to a local `./bin/` directory
- Supports `darwin`/`linux` with `amd64`/`arm64`

Part 2 — Kubernetes cluster:
- Detects existing cluster via `kubectl cluster-info`
- Offers to reuse it or create a new kind cluster
- Writes `.cluster-mode` marker (`"kind"` or `"existing"`)

Does not install operators or Helm repos — those are walkthrough steps in `guide.sh`.

### guide.sh

The interactive guide. Sources `runner.sh` for terminal UX, then walks through all five steps with explanatory text, `prompt_run` commands, and spinners for slow operations.

Key behaviors:
- Detects existing Kubernetes clusters and offers to reuse them
- Falls back to `setup.sh` if no cluster is found
- Tracks cluster origin via `.cluster-mode` for appropriate cleanup
- Idempotent operator installs (checks before installing cert-manager/CNPG)
- Cleanup prompt at the end (kind cluster deletion vs. selective resource removal)

### runner.sh

Reusable terminal UX framework, sourced by `guide.sh`. Provides:

- Brand colors — teal (`\033[38;5;30m`) and orange (`\033[38;5;172m`) from the pgEdge palette
- `header` — teal bordered section headers
- `show_cmd` / `prompt_run` — shows a command with orange `$` prefix, waits for Enter, runs it with framed output
- `prompt_continue` — simple "Press Enter to continue" gate
- `start_spinner` / `stop_spinner` — braille dot animation for long-running operations
- `explain` / `info` — plain text and green success messages

This file is standalone and could be reused for other interactive guides.

### values/

Three progressive Helm values files. Each builds on the previous:

| File | Nodes | Instances | What changes |
|------|-------|-----------|-------------|
| `step1-single-primary.yaml` | n1 | 1 | Baseline deployment |
| `step2-with-replicas.yaml` | n1 | 2 | Adds sync standby, `dataDurability: required` |
| `step3-multi-master.yaml` | n1, n2 | 2, 1 | Adds n2 with `bootstrap.mode: spock`, `sourceNode: n1` |

The walkthrough uses `helm install` for step 1, then `helm upgrade` for steps 2 and 3.

## Running the Walkthrough

### Interactive Guide (`curl ... | bash`)

The primary entrypoint. Downloads the walkthrough files into a self-contained directory, installs tools locally, creates a kind cluster, and launches the interactive guide. Only Docker is required.

```bash
curl -fsSL https://raw.githubusercontent.com/pgEdge/pgedge-helm/main/examples/walkthrough/install.sh | bash
```

What happens:
1. `install.sh` downloads scripts, values files, and `docs/walkthrough.md` into `pgedge-walkthrough/` (no git clone — individual file downloads)
2. `setup.sh` installs missing tools (`kind`, `kubectl`, `helm`, `kubectl-cnpg`) into a local `./bin/` directory and creates a kind cluster
3. User chooses: run the interactive guide (`guide.sh`), or exit and follow `docs/walkthrough.md` manually

### GitHub Codespaces

Open a Codespace with the walkthrough devcontainer:

```
https://github.com/codespaces/new?repo=pgEdge/pgedge-helm&devcontainer_path=.devcontainer/walkthrough/devcontainer.json
```

The devcontainer ([`.devcontainer/walkthrough/`](../../.devcontainer/walkthrough/)) handles everything:

1. Base image — Ubuntu with Docker-in-Docker, kubectl, and Helm pre-installed
2. `post-create.sh` — installs `kind` and the `kubectl-cnpg` plugin, then runs `setup.sh` to create the kind cluster
3. `postAttachCommand` — opens `docs/walkthrough.md` in the editor
4. Runme extension — pre-installed so users can run markdown code blocks directly

Users can run the code blocks in `walkthrough.md` directly from the editor, or switch to the interactive guide: `bash examples/walkthrough/guide.sh`.

### From a cloned repo

The walkthrough is designed to work without cloning the repo, but if you already have it cloned you can run the pieces directly:

```bash
# Run the interactive guide (calls setup.sh if no cluster is detected)
bash examples/walkthrough/guide.sh

# Or run setup and guide separately
bash examples/walkthrough/setup.sh
bash examples/walkthrough/guide.sh
```

Note that `setup.sh` installs tools to `examples/walkthrough/bin/` — you'll need to add that to your PATH or already have `kind`, `kubectl`, `helm`, and `kubectl-cnpg` installed.
