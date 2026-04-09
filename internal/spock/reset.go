// internal/spock/reset.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// ResetSpock drops and recreates Spock on every node connection.
// This follows the Control Plane pattern: backup repsets, nuke spock,
// reinitialize with correct config, restore repsets.
func ResetSpock(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	for _, node := range cfg.Nodes {
		conn := conns[node.Name]
		if err := resetNode(ctx, conn, node, cfg.DBName, cfg.PgEdgeUser); err != nil {
			return fmt.Errorf("reset spock on %s: %w", node.Name, err)
		}
	}
	return nil
}

func resetNode(ctx context.Context, conn *pgxpool.Pool, node config.Node, dbName, pgedgeUser string) error {
	var spockExists bool
	err := conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'spock')",
	).Scan(&spockExists)
	if err != nil {
		return fmt.Errorf("check spock extension on %s: %w", node.Name, err)
	}

	if spockExists {
		if err := BackupRepsets(ctx, conn, node.Name); err != nil {
			slog.Warn("backup repsets failed (continuing with reset)", "node", node.Name, "error", err)
		}
	}

	_, err = conn.Exec(ctx, "DROP EXTENSION IF EXISTS spock CASCADE")
	if err != nil {
		return fmt.Errorf("drop spock extension on %s: %w", node.Name, err)
	}
	slog.Info("dropped spock extension", "node", node.Name)

	_, err = conn.Exec(ctx,
		"SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_type = 'logical'")
	if err != nil {
		slog.Warn("drop replication slots failed (continuing)", "node", node.Name, "error", err)
	}

	_, err = conn.Exec(ctx,
		"SELECT pg_replication_origin_drop(roname) FROM pg_replication_origin")
	if err != nil {
		slog.Warn("drop replication origins failed (continuing)", "node", node.Name, "error", err)
	}

	_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS spock")
	if err != nil {
		return fmt.Errorf("create spock extension on %s: %w", node.Name, err)
	}

	dsn := fmt.Sprintf("host=%s dbname=%s user=%s %s port=5432",
		node.Hostname, dbName, pgedgeUser, sslSettings)
	_, err = conn.Exec(ctx, "SELECT spock.node_create($1, $2)", node.Name, dsn)
	if err != nil {
		return fmt.Errorf("create spock node on %s: %w", node.Name, err)
	}
	slog.Info("recreated spock node", "node", node.Name)

	if err := RestoreRepsets(ctx, conn, node.Name); err != nil {
		slog.Warn("restore repsets failed (continuing)", "node", node.Name, "error", err)
	}

	return nil
}
