// internal/spock/user.go
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

// PgEdgeUser ensures the pgedge replication role exists on a node.
type PgEdgeUser struct {
	node       config.Node
	dbName     string
	adminUser  string
	pgedgeUser string
	conn       *pgxpool.Pool
	status     resource.Status
}

func NewPgEdgeUser(node config.Node, dbName, adminUser, pgedgeUser string, conn *pgxpool.Pool) *PgEdgeUser {
	return &PgEdgeUser{
		node:       node,
		dbName:     dbName,
		adminUser:  adminUser,
		pgedgeUser: pgedgeUser,
		conn:       conn,
	}
}

func (u *PgEdgeUser) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeUser, ID: u.node.Name}
}

func (u *PgEdgeUser) Dependencies() []resource.Identifier { return nil }

func (u *PgEdgeUser) Refresh(ctx context.Context) error {
	var exists bool
	err := u.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", u.pgedgeUser,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("inspect pgedge user on %s: %w", u.node.Name, err)
	}
	u.status = resource.Status{Exists: exists}
	return nil
}

func (u *PgEdgeUser) Status() resource.Status { return u.status }

func (u *PgEdgeUser) Create(ctx context.Context) error {
	tx, err := u.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx on %s: %w", u.node.Name, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT spock.repair_mode('True')")
	if err != nil {
		return fmt.Errorf("repair mode on %s: %w", u.node.Name, err)
	}

	stmt := fmt.Sprintf("CREATE ROLE %s WITH LOGIN SUPERUSER REPLICATION", u.pgedgeUser)
	_, err = tx.Exec(ctx, stmt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42710" {
			slog.Info("pgedge user already exists", "node", u.node.Name)
			return nil
		}
		return fmt.Errorf("create pgedge user on %s: %w", u.node.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pgedge user on %s: %w", u.node.Name, err)
	}
	slog.Info("created pgedge user", "node", u.node.Name)
	return nil
}

func (u *PgEdgeUser) Delete(ctx context.Context) error {
	return nil // Never delete the pgedge user
}
