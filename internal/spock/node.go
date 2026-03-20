// internal/spock/node.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

const sslSettings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"

// SpockNode manages a Spock node on a PostgreSQL instance.
type SpockNode struct {
	node       config.Node
	dbName     string
	adminUser  string
	pgedgeUser string
	conn       *pgxpool.Pool
	status     resource.Status
}

func NewSpockNode(node config.Node, dbName, adminUser, pgedgeUser string, conn *pgxpool.Pool) *SpockNode {
	return &SpockNode{
		node:       node,
		dbName:     dbName,
		adminUser:  adminUser,
		pgedgeUser: pgedgeUser,
		conn:       conn,
	}
}

func (n *SpockNode) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeNode, ID: n.node.Name}
}

func (n *SpockNode) Dependencies() []resource.Identifier {
	return []resource.Identifier{{Type: ResourceTypeUser, ID: n.node.Name}}
}

func (n *SpockNode) Refresh(ctx context.Context) error {
	var localName string
	err := n.conn.QueryRow(ctx,
		"SELECT n.node_name FROM spock.node n JOIN spock.local_node ln ON n.node_id = ln.node_id",
	).Scan(&localName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			n.status = resource.Status{Exists: false}
			return nil
		}
		return fmt.Errorf("inspect spock node on %s: %w", n.node.Name, err)
	}

	if localName != n.node.Name {
		n.status = resource.Status{
			Exists:        true,
			NeedsRecreate: true,
			Reason:        fmt.Sprintf("local node name %q != configured %q (CNPG bootstrap)", localName, n.node.Name),
		}
		return nil
	}

	n.status = resource.Status{Exists: true}
	return nil
}

func (n *SpockNode) Status() resource.Status { return n.status }

func (n *SpockNode) Create(ctx context.Context) error {
	tx, err := n.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx on %s: %w", n.node.Name, err)
	}
	defer tx.Rollback(ctx)

	dsn := fmt.Sprintf("host=%s dbname=%s user=%s %s port=5432",
		n.node.Hostname, n.dbName, n.pgedgeUser, sslSettings)

	_, err = tx.Exec(ctx, "SELECT spock.repair_mode('True')")
	if err != nil {
		return fmt.Errorf("enable repair mode on %s: %w", n.node.Name, err)
	}

	_, err = tx.Exec(ctx, `
		SELECT spock.node_create(
			node_name := $1,
			dsn := $2
		)
		WHERE $1 NOT IN (SELECT node_name FROM spock.node)`,
		n.node.Name, dsn)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42710" {
			slog.Info("spock node already exists", "node", n.node.Name)
		} else {
			return fmt.Errorf("create spock node %s: %w", n.node.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit spock node %s: %w", n.node.Name, err)
	}
	slog.Info("created spock node", "node", n.node.Name)

	// Restore repsets after node recreation (CNPG-bootstrapped nodes)
	// Must happen after Create, not Delete — Spock functions require a node to exist.
	// Matches Python flow: backup → drop → create → restore
	if n.status.NeedsRecreate {
		if err := RestoreRepsets(ctx, n.conn, n.node.Name); err != nil {
			slog.Warn("failed to restore repsets", "node", n.node.Name, "error", err)
		}
	}
	return nil
}

func (n *SpockNode) Delete(ctx context.Context) error {
	if n.status.NeedsRecreate {
		return n.deleteAll(ctx)
	}
	return n.deleteOne(ctx)
}

// deleteAll wipes all Spock state on this connection (CNPG bootstrap recovery).
func (n *SpockNode) deleteAll(ctx context.Context) error {
	if err := BackupRepsets(ctx, n.conn, n.node.Name); err != nil {
		return err
	}

	tx, err := n.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx on %s: %w", n.node.Name, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		SELECT spock.repair_mode('True');
		SELECT spock.sub_drop(sub_name, true) FROM spock.subscription;
		SELECT spock.node_drop(node_name, true) FROM spock.node;
	`)
	if err != nil {
		return fmt.Errorf("drop all spock state on %s: %w", n.node.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit drop all spock state on %s: %w", n.node.Name, err)
	}
	slog.Info("dropped all spock state", "node", n.node.Name)
	return nil
}

// deleteOne drops a single node reference (orphan cleanup).
func (n *SpockNode) deleteOne(ctx context.Context) error {
	tx, err := n.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx on %s: %w", n.node.Name, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `SELECT spock.repair_mode('True')`)
	if err != nil {
		return fmt.Errorf("enable repair mode on %s: %w", n.node.Name, err)
	}

	_, err = tx.Exec(ctx, `SELECT spock.node_drop($1, true)`, n.node.Name)
	if err != nil {
		return fmt.Errorf("drop spock node %s: %w", n.node.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit drop spock node %s: %w", n.node.Name, err)
	}
	slog.Info("dropped spock node", "node", n.node.Name)
	return nil
}
