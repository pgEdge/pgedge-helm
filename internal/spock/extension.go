// internal/spock/extension.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

const (
	extensionPollInterval = 2 * time.Second

	// extensionUpdateGrace is how long to wait for Spock's manager worker to
	// update the extension itself before running ALTER EXTENSION here.
	extensionUpdateGrace = 60 * time.Second
)

// extensionVersionQuery returns the installed Spock extension version, the
// version the server's binaries provide, and whether PostgreSQL knows an
// update path between the two. No row means Spock isn't installed.
const extensionVersionQuery = `
	SELECT e.extversion,
	       a.default_version,
	       EXISTS (
	         SELECT 1 FROM pg_extension_update_paths('spock') p
	         WHERE p.source = e.extversion
	           AND p.target = a.default_version
	           AND p.path IS NOT NULL
	       )
	FROM pg_extension e
	JOIN pg_available_extensions a ON a.name = e.extname
	WHERE e.extname = 'spock'`

type extensionAction int

const (
	// extensionCurrent: the catalog matches the binaries.
	extensionCurrent extensionAction = iota
	// extensionNeedsUpdate: the binaries are newer and there is an update path.
	extensionNeedsUpdate
	// extensionNoPath: the versions differ and there is no update path, e.g.
	// the image was rolled back to an older Spock than the catalog.
	extensionNoPath
)

func decideExtensionAction(installed, available string, hasPath bool) extensionAction {
	switch {
	case installed == available:
		return extensionCurrent
	case hasPath:
		return extensionNeedsUpdate
	default:
		return extensionNoPath
	}
}

// EnsureExtensionsCurrent makes sure the Spock extension's catalog on every
// node matches the Spock binaries the node runs, before anything reads or
// changes Spock state.
//
// After an image change that moves to a newer Spock (for example 5.0.x to
// 6.0), Spock's manager worker runs ALTER EXTENSION spock UPDATE on the
// primary by itself shortly after start-up. This waits for that, and runs the
// update here if it hasn't happened within extensionUpdateGrace. Nodes
// without the extension are skipped; CREATE EXTENSION installs the current
// version on them. So are nodes this run resets (resetSpock, or a node just
// restored from a backup), since the reset drops and recreates the extension.
func EnsureExtensionsCurrent(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	if cfg.ResetSpock {
		return nil
	}
	for _, node := range cfg.Nodes {
		if node.Bootstrap.Mode == "cnpg" {
			continue
		}
		if err := ensureExtensionCurrent(ctx, conns[node.Name], node.Name); err != nil {
			return err
		}
	}
	return nil
}

func ensureExtensionCurrent(ctx context.Context, conn *pgxpool.Pool, nodeName string) error {
	deadline := time.Now().Add(extensionUpdateGrace)
	logged := false

	for {
		var installed, available string
		var hasPath bool
		err := conn.QueryRow(ctx, extensionVersionQuery).Scan(&installed, &available, &hasPath)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check spock extension version on %s: %w", nodeName, err)
		}

		switch decideExtensionAction(installed, available, hasPath) {
		case extensionCurrent:
			if logged {
				slog.Info("spock extension updated", "node", nodeName, "version", installed)
			}
			return nil
		case extensionNoPath:
			return fmt.Errorf("spock extension on %s is at version %s but the server provides %s, "+
				"and there is no update path between them; check the node's image "+
				"(downgrading Spock is not supported)", nodeName, installed, available)
		}

		if !logged {
			slog.Info("waiting for spock extension update",
				"node", nodeName, "installed", installed, "available", available)
			logged = true
		}

		if time.Now().After(deadline) {
			if err := updateExtension(ctx, conn); err != nil {
				// Spock's manager worker may be updating it at the same time.
				// Check again on the next round rather than failing.
				slog.Warn("ALTER EXTENSION spock UPDATE failed, retrying",
					"node", nodeName, "error", err)
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for the spock extension on %s to update from %s to %s: %w",
				nodeName, installed, available, ctx.Err())
		case <-time.After(extensionPollInterval):
		}
	}
}

func updateExtension(ctx context.Context, conn *pgxpool.Pool) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Keep the update node-local. Each node runs its own update for the
	// binaries it has.
	if _, err := tx.Exec(ctx, "SELECT set_config('spock.enable_ddl_replication', 'off', true)"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "ALTER EXTENSION spock UPDATE"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
