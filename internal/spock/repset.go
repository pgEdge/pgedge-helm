// internal/spock/repset.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupRepsets backs up Spock replication sets before dropping a node.
// Corresponds to Python backup_spock_repsets().
func BackupRepsets(ctx context.Context, conn *pgxpool.Pool, nodeName string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin repset backup on %s: %w", nodeName, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SET log_statement = 'none'")
	if err != nil {
		return fmt.Errorf("set log_statement on %s: %w", nodeName, err)
	}

	_, err = tx.Exec(ctx, `
		SET spock.enable_ddl_replication = off;
		DROP TABLE IF EXISTS spock.replication_set_table_backup;
		DROP TABLE IF EXISTS spock.replication_set_backup;
	`)
	if err != nil {
		return fmt.Errorf("cleanup old backup on %s: %w", nodeName, err)
	}

	_, err = tx.Exec(ctx, `
		SET spock.enable_ddl_replication = off;
		CREATE TABLE spock.replication_set_backup AS SELECT * FROM spock.replication_set;
		CREATE TABLE spock.replication_set_table_backup AS SELECT * FROM spock.replication_set_table;
	`)
	if err != nil {
		return fmt.Errorf("backup repsets on %s: %w", nodeName, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repset backup on %s: %w", nodeName, err)
	}
	slog.Info("backed up repsets", "node", nodeName)
	return nil
}

// RestoreRepsets restores Spock replication sets after recreating a node.
// Corresponds to Python restore_spock_repsets(). Always cleans up backup tables.
func RestoreRepsets(ctx context.Context, conn *pgxpool.Pool, nodeName string) error {
	// Check if backup exists
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'spock' AND table_name = 'replication_set_backup'
		)
	`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check repset backup on %s: %w", nodeName, err)
	}
	if !exists {
		slog.Info("no repset backup to restore", "node", nodeName)
		return nil
	}

	// Cleanup backup tables after a successful restore
	defer cleanupRepsetBackup(ctx, conn, nodeName)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin repset restore on %s: %w", nodeName, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SET log_statement = 'none'")
	if err != nil {
		return fmt.Errorf("set log_statement on %s: %w", nodeName, err)
	}

	// Recreate custom replication sets (skip built-in ones)
	_, err = tx.Exec(ctx, `
		SELECT spock.repset_create(
			rs.set_name, rs.replicate_insert, rs.replicate_update,
			rs.replicate_delete, rs.replicate_truncate
		)
		FROM spock.replication_set_backup rs
		WHERE rs.set_name NOT IN ('default', 'default_insert_only', 'ddl_sql')
	`)
	if err != nil {
		return fmt.Errorf("recreate repsets on %s: %w", nodeName, err)
	}

	// Re-add tables to replication sets.
	// Collect all rows first — pgx can't execute on a tx while rows are open.
	type repsetTable struct {
		setName, tableName string
		attList, rowFilter *string
	}
	rows, err := tx.Query(ctx, `
		SELECT rs.set_name, rst.set_reloid::regclass::text, rst.set_att_list, rst.set_row_filter
		FROM spock.replication_set_table_backup rst
		LEFT JOIN spock.replication_set_backup rs ON rst.set_id = rs.set_id
		WHERE EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.oid = rst.set_reloid AND c.relkind IN ('r', 'v')
		)
	`)
	if err != nil {
		return fmt.Errorf("query repset tables on %s: %w", nodeName, err)
	}
	var tables []repsetTable
	for rows.Next() {
		var t repsetTable
		if err := rows.Scan(&t.setName, &t.tableName, &t.attList, &t.rowFilter); err != nil {
			slog.Warn("scan repset table row", "node", nodeName, "error", err)
			continue
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read repset table backup on %s: %w", nodeName, err)
	}

	for _, t := range tables {
		_, err := tx.Exec(ctx, "SELECT spock.repset_add_table($1, $2, false, $3, $4)",
			t.setName, t.tableName, t.attList, t.rowFilter)
		if err != nil {
			slog.Warn("add table to repset", "node", nodeName, "table", t.tableName, "set", t.setName, "error", err)
			continue
		}
		slog.Info("restored table to repset", "node", nodeName, "table", t.tableName, "set", t.setName)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repset restore on %s: %w", nodeName, err)
	}
	slog.Info("restored repsets", "node", nodeName)
	return nil
}

// cleanupRepsetBackup drops the backup tables regardless of restore success/failure.
func cleanupRepsetBackup(ctx context.Context, conn *pgxpool.Pool, nodeName string) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		slog.Warn("begin repset cleanup", "node", nodeName, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		SET log_statement = 'none';
		SET spock.enable_ddl_replication = off;
		DROP TABLE IF EXISTS spock.replication_set_table_backup;
		DROP TABLE IF EXISTS spock.replication_set_backup;
	`)
	if err != nil {
		slog.Warn("cleanup repset backup tables", "node", nodeName, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Warn("commit repset cleanup", "node", nodeName, "error", err)
	}
}
