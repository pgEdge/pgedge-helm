// cmd/init-spock/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
		pool, err := pg.ConnectPool(ctx, node.Hostname, cfg.DBName, cfg.AdminUser)
		if err != nil {
			return err
		}
		defer pool.Close()
		conns[node.Name] = pool
	}

	// Step 3: Reset Spock if requested (Barman restore scenarios)
	if cfg.ResetSpock {
		slog.Info("resetSpock enabled — dropping and recreating spock on all nodes")
		if err := spock.ResetSpock(ctx, cfg, conns); err != nil {
			return err
		}
	}

	// Step 4: Reconcile Spock resources
	return resource.Reconcile(ctx, spock.NewReconciler(cfg, conns))
}
