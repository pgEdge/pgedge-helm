// internal/spock/reset.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	// Slot names of this database's subscriptions on this node, whose
	// replication origins are dropped below.
	var subSlots []string

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
		subSlots, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("list subscriptions on %s: %w", node.Name, err)
		}
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

	_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS spock")
	if err != nil {
		return fmt.Errorf("create spock extension on %s: %w", node.Name, err)
	}

	// Replication origins have no database column, and Spock in other
	// databases on this server has its own. Ours are named after the slots of
	// this database's subscriptions on this node: the ones in the catalog that
	// was just dropped, and the ones the reconcile creates again, whose names
	// Spock generates. Table sync workers use the slot name plus "_%08x", and
	// can leave that origin behind if they stop mid-copy. Slot names only
	// contain [a-z0-9_], so they are safe in a regular expression.
	var peers, subNames []string
	for _, peer := range cfg.Nodes {
		if peer.Name != node.Name {
			peers = append(peers, peer.Name)
			subNames = append(subNames, spockSubName(peer.Name, node.Name))
		}
	}
	rows, err := conn.Query(ctx, `
		WITH ours AS (
			SELECT unnest($1::text[]) AS slot
			UNION
			SELECT spock.spock_gen_slot_name(current_database()::name, p::name, s::name)::text
			  FROM unnest($2::text[], $3::text[]) AS t(p, s))
		SELECT roname
		  FROM pg_replication_origin
		 WHERE EXISTS (SELECT 1 FROM ours
		                WHERE roname = slot OR roname ~ ('^' || slot || '_[0-9a-f]{8}$'))`,
		subSlots, peers, subNames)
	var origins []string
	if err == nil {
		origins, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		slog.Warn("list replication origins failed (continuing)", "node", node.Name, "error", err)
	}
	// Drop them one at a time: an apply worker that has not exited yet still
	// holds its origin, and that must not stop the others from being dropped.
	// Wait for such a worker to let go, because a disabled subscription (used
	// to add a node) cannot be created while its origin is still there.
	deadline := time.Now().Add(originDropWait)
	for _, origin := range origins {
		if err := dropOrigin(ctx, conn, origin, deadline); err != nil {
			slog.Warn("drop replication origin failed (continuing)", "node", node.Name, "origin", origin, "error", err)
		}
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

// originDropWait is how long resetNode waits for workers to release their
// replication origins after Spock is dropped.
const originDropWait = 30 * time.Second

// dropOrigin drops a replication origin, retrying while a worker still holds
// it (SQLSTATE 55006, object in use) until the deadline.
func dropOrigin(ctx context.Context, conn *pgxpool.Pool, origin string, deadline time.Time) error {
	for {
		_, err := conn.Exec(ctx, "SELECT pg_replication_origin_drop($1)", origin)
		var pgErr *pgconn.PgError
		if err == nil || !errors.As(err, &pgErr) || pgErr.Code != "55006" || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
