// cmd/init-spock/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/cluster"
	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/pg"
	"github.com/pgEdge/pgedge-helm/internal/resource"
	"github.com/pgEdge/pgedge-helm/internal/spock"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("init-spock failed", "error", err)
		os.Exit(1)
	}
	slog.Info("spock configuration successfully updated")
}

func run(ctx context.Context) error {
	cfg, err := config.Load("/config/pgedge.yaml")
	if err != nil {
		return err
	}

	// Apply timeout if configured. Use 30s less than the Kubernetes
	// activeDeadlineSeconds so the process can log errors gracefully
	// before the kubelet sends SIGTERM.
	if timeoutStr := os.Getenv("INIT_SPOCK_TIMEOUT"); timeoutStr != "" {
		timeoutSec, err := strconv.Atoi(timeoutStr)
		if err == nil && timeoutSec > 0 {
			gracePeriod := 30
			if timeoutSec <= gracePeriod {
				gracePeriod = 0
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec-gracePeriod)*time.Second)
			defer cancel()
		}
	}

	slog.Info("configuring spock", "nodes", len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		slog.Info("node", "name", node.Name, "hostname", node.Hostname,
			"bootstrap_mode", node.Bootstrap.Mode)
	}

	// Step 1: Wait for CNPG clusters
	if err := cluster.WaitForAll(ctx, cfg.Namespace, cfg.AppName); err != nil {
		return err
	}

	// Step 2: Wait for nodes and establish connection pools
	conns := make(map[string]*pgxpool.Pool)
	for _, node := range cfg.Nodes {
		if err := pg.WaitReady(ctx, node.Hostname, node.InternalHostname, cfg.DBName, cfg.AdminUser); err != nil {
			return err
		}
		pool, err := pg.ConnectPool(ctx, node.Hostname, node.InternalHostname, cfg.DBName, cfg.AdminUser)
		if err != nil {
			return err
		}
		defer pool.Close()
		conns[node.Name] = pool
	}

	// Step 3: Before changing anything, reject adding a node with a
	// different Spock major version. Then make sure each node's Spock
	// catalog matches its binaries (e.g. after an image change from Spock
	// 5.0.x to 6.0), that nodes which need native slot sync for their
	// standbys have it, and warn about settings that would stall
	// replication.
	if err := spock.ValidateAddNodeVersions(ctx, cfg, conns); err != nil {
		return err
	}
	if err := spock.EnsureExtensionsCurrent(ctx, cfg, conns); err != nil {
		return err
	}
	if err := spock.ValidateNativeSlotSync(ctx, cfg, conns); err != nil {
		return err
	}
	spock.CheckStandbySlotNames(ctx, cfg, conns)

	// Step 4: Reset Spock state where needed
	if cfg.ResetSpock {
		slog.Info("resetSpock enabled — dropping and recreating spock on all nodes")
		if err := spock.ResetSpock(ctx, cfg, conns); err != nil {
			return err
		}
	} else {
		if err := spock.ResetBootstrappedNodes(ctx, cfg, conns); err != nil {
			return err
		}
	}

	// Step 5: Reconcile Spock resources
	if err := resource.Reconcile(ctx, spock.NewReconciler(cfg, conns)); err != nil {
		return err
	}

	// Step 6: On PostgreSQL 17+ with Spock 6.0, make sure every Spock slot
	// has the failover flag so native slot sync copies it to standbys. Slots
	// carried over from Spock 5.0.x that were in use during the extension
	// update don't have it yet.
	return spock.EnableSlotFailover(ctx, cfg, conns)
}
