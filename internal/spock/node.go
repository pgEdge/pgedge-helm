// internal/spock/node.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// pgCodeDuplicateObject is the PostgreSQL error code for "duplicate object".
const pgCodeDuplicateObject = "42710"

const sslSettings = "sslcert=/projected/pgedge/certificates/tls.crt sslkey=/projected/pgedge/certificates/tls.key sslmode=require"

// SpockNode manages a Spock node on a PostgreSQL instance.
type SpockNode struct {
	node       config.Node
	dbName     string
	pgedgeUser string
	conn       *pgxpool.Pool
	status     resource.Status
}

func NewSpockNode(node config.Node, dbName, pgedgeUser string, conn *pgxpool.Pool) *SpockNode {
	return &SpockNode{
		node:       node,
		dbName:     dbName,
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
	var exists bool
	err := n.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM spock.node n JOIN spock.local_node ln ON n.node_id = ln.node_id WHERE n.node_name = $1)",
		n.node.Name,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("inspect spock node on %s: %w", n.node.Name, err)
	}
	n.status = resource.Status{Exists: exists}
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
		if errors.As(err, &pgErr) && pgErr.Code == pgCodeDuplicateObject {
			slog.Info("spock node already exists", "node", n.node.Name)
			return nil
		}
		return fmt.Errorf("create spock node %s: %w", n.node.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit spock node %s: %w", n.node.Name, err)
	}
	slog.Info("created spock node", "node", n.node.Name)
	return nil
}

func (n *SpockNode) Delete(ctx context.Context) error {
	return n.deleteOne(ctx)
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
