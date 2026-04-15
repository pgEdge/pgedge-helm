// internal/spock/repset.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// replicationSet holds an in-memory snapshot of a spock replication set.
type replicationSet struct {
	name             string
	replicateInsert  bool
	replicateUpdate  bool
	replicateDelete  bool
	replicateTruncate bool
}

// replicationSetTable holds an in-memory snapshot of a table assignment.
type replicationSetTable struct {
	setName   string
	tableName string
	attList   *string
	rowFilter *string
}

// repsetSnapshot holds the in-memory backup of replication set configuration.
type repsetSnapshot struct {
	sets   []replicationSet
	tables []replicationSetTable
}

// backupRepsets reads replication set configuration into memory.
// This must be called before DROP EXTENSION since the spock schema
// is destroyed by the cascade.
func backupRepsets(ctx context.Context, conn *pgxpool.Pool, nodeName string) (*repsetSnapshot, error) {
	snap := &repsetSnapshot{}

	// Read all replication sets (including built-in).
	// Built-in sets are recreated by CREATE EXTENSION, but we need their
	// IDs to map table assignments. Only custom sets get repset_create'd.
	rows, err := conn.Query(ctx, `
		SELECT set_name, replicate_insert, replicate_update, replicate_delete, replicate_truncate
		FROM spock.replication_set
		ORDER BY set_id`)
	if err != nil {
		return nil, fmt.Errorf("query replication sets on %s: %w", nodeName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var s replicationSet
		if err := rows.Scan(&s.name, &s.replicateInsert, &s.replicateUpdate, &s.replicateDelete, &s.replicateTruncate); err != nil {
			return nil, fmt.Errorf("scan replication set on %s: %w", nodeName, err)
		}
		snap.sets = append(snap.sets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read replication sets on %s: %w", nodeName, err)
	}

	// Read replication set table assignments.
	trows, err := conn.Query(ctx, `
		SELECT rs.set_name, rst.set_reloid::regclass::text, rst.set_att_list, rst.set_row_filter
		FROM spock.replication_set_table rst
		JOIN spock.replication_set rs ON rst.set_id = rs.set_id
		WHERE EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.oid = rst.set_reloid AND c.relkind IN ('r', 'v')
		)
		ORDER BY rst.set_id, rst.set_reloid`)
	if err != nil {
		return nil, fmt.Errorf("query replication set tables on %s: %w", nodeName, err)
	}
	defer trows.Close()
	for trows.Next() {
		var t replicationSetTable
		if err := trows.Scan(&t.setName, &t.tableName, &t.attList, &t.rowFilter); err != nil {
			return nil, fmt.Errorf("scan replication set table on %s: %w", nodeName, err)
		}
		snap.tables = append(snap.tables, t)
	}
	if err := trows.Err(); err != nil {
		return nil, fmt.Errorf("read replication set tables on %s: %w", nodeName, err)
	}

	slog.Info("backed up repsets to memory", "node", nodeName, "sets", len(snap.sets), "tables", len(snap.tables))
	return snap, nil
}

// restoreRepsets writes the in-memory replication set snapshot back to the database.
// Must be called after the spock extension and local node have been recreated.
func restoreRepsets(ctx context.Context, conn *pgxpool.Pool, nodeName string, snap *repsetSnapshot) error {
	if snap == nil || (len(snap.sets) == 0 && len(snap.tables) == 0) {
		slog.Info("no repsets to restore", "node", nodeName)
		return nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin repset restore on %s: %w", nodeName, err)
	}
	defer tx.Rollback(ctx)

	// Recreate custom replication sets (built-in ones already exist after CREATE EXTENSION).
	builtinSets := map[string]bool{"default": true, "default_insert_only": true, "ddl_sql": true}
	for _, s := range snap.sets {
		if builtinSets[s.name] {
			continue
		}
		_, err := tx.Exec(ctx,
			"SELECT spock.repset_create($1, $2, $3, $4, $5)",
			s.name, s.replicateInsert, s.replicateUpdate, s.replicateDelete, s.replicateTruncate)
		if err != nil {
			return fmt.Errorf("recreate repset %q on %s: %w", s.name, nodeName, err)
		}
	}

	// Re-add tables to replication sets.
	for _, t := range snap.tables {
		_, err := tx.Exec(ctx,
			"SELECT spock.repset_add_table($1, $2, false, $3, $4)",
			t.setName, t.tableName, t.attList, t.rowFilter)
		if err != nil {
			return fmt.Errorf("add table %q to repset %q on %s: %w", t.tableName, t.setName, nodeName, err)
		}
		slog.Info("restored table to repset", "node", nodeName, "table", t.tableName, "set", t.setName)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repset restore on %s: %w", nodeName, err)
	}
	slog.Info("restored repsets", "node", nodeName)
	return nil
}
