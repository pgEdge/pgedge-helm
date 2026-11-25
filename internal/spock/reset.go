// internal/spock/reset.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// ResetSpock drops and recreates Spock on every node connection.
// This follows the Control Plane pattern: backup repsets, nuke spock,
// reinitialize with correct config, restore repsets.
func ResetSpock(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	for _, node := range cfg.Nodes {
		conn := conns[node.Name]
		if err := resetNode(ctx, conn, node, cfg); err != nil {
			return fmt.Errorf("reset spock on %s: %w", node.Name, err)
		}
	}
	return nil
}

// ResetBootstrappedNodes resets Spock on nodes bootstrapped via CNPG restore.
// These nodes have stale spock catalog state from the backup source.
func ResetBootstrappedNodes(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	for _, node := range cfg.Nodes {
		if node.Bootstrap.Mode != "cnpg" {
			continue
		}
		slog.Info("resetting CNPG-bootstrapped node", "node", node.Name)
		conn := conns[node.Name]
		if err := resetNode(ctx, conn, node, cfg); err != nil {
			return fmt.Errorf("reset bootstrapped node %s: %w", node.Name, err)
		}
	}
	return nil
}

func resetNode(ctx context.Context, conn *pgxpool.Pool, node config.Node, cfg *config.Config) error {
	var spockExists bool
	err := conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'spock')",
	).Scan(&spockExists)
	if err != nil {
		return fmt.Errorf("check spock extension on %s: %w", node.Name, err)
	}

	// Replication origins have no database column, and Spock in other
	// databases on this server has its own. Ours are named after the slots of
	// this database's subscriptions on this node: the ones in the catalog
	// being dropped, and the ones the reconcile creates again afterwards.
	origins := []string{}
	for _, peer := range cfg.Nodes {
		if peer.Name != node.Name {
			origins = append(origins, spockSlotName(cfg.DBName, peer.Name, node.Name))
		}
	}

	// Backup repsets into memory before nuking — the spock schema is
	// destroyed by DROP EXTENSION CASCADE, so SQL-table backups won't survive.
	var snap *repsetSnapshot
	if spockExists {
		snap, err = backupRepsets(ctx, conn, node.Name)
		if err != nil {
			return fmt.Errorf("backup repsets on %s: %w", node.Name, err)
		}
		rows, err := conn.Query(ctx, "SELECT sub_slot_name FROM spock.subscription")
		if err != nil {
			return fmt.Errorf("list subscriptions on %s: %w", node.Name, err)
		}
		subSlots, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("list subscriptions on %s: %w", node.Name, err)
		}
		origins = append(origins, subSlots...)
	}

	_, err = conn.Exec(ctx, "DROP EXTENSION IF EXISTS spock CASCADE")
	if err != nil {
		return fmt.Errorf("drop spock extension on %s: %w", node.Name, err)
	}
	slog.Info("dropped spock extension", "node", node.Name)

	// Terminate active walsenders on this database's spock slots before dropping.
	_, err = conn.Exec(ctx, `
		SELECT pg_terminate_backend(active_pid)
		  FROM pg_replication_slots
		 WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%' AND active
		   AND database = current_database()`)
	if err != nil {
		slog.Warn("terminate walsenders failed (continuing)", "node", node.Name, "error", err)
	}

	_, err = conn.Exec(ctx, `
		SELECT pg_drop_replication_slot(slot_name)
		  FROM pg_replication_slots
		 WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%'
		   AND database = current_database()`)
	if err != nil {
		slog.Warn("drop replication slots failed (continuing)", "node", node.Name, "error", err)
	}

	_, err = conn.Exec(ctx, `
		SELECT pg_replication_origin_drop(roname)
		  FROM pg_replication_origin
		 WHERE roname = ANY($1)`, origins)
	if err != nil {
		slog.Warn("drop replication origins failed (continuing)", "node", node.Name, "error", err)
	}

	_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS spock")
	if err != nil {
		return fmt.Errorf("create spock extension on %s: %w", node.Name, err)
	}

	dsn := fmt.Sprintf("host=%s dbname=%s user=%s %s port=5432",
		node.Hostname, cfg.DBName, cfg.PgEdgeUser, sslSettings)
	_, err = conn.Exec(ctx, "SELECT spock.node_create($1, $2)", node.Name, dsn)
	if err != nil {
		return fmt.Errorf("create spock node on %s: %w", node.Name, err)
	}
	slog.Info("recreated spock node", "node", node.Name)

	if err := restoreRepsets(ctx, conn, node.Name, snap); err != nil {
		return fmt.Errorf("restore repsets on %s: %w", node.Name, err)
	}

	return nil
}
