// internal/spock/subscription.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// Subscription manages a Spock subscription between two nodes.
// Executes on the subscriber (dst) node's connection.
type Subscription struct {
	src        config.Node
	dst        config.Node
	dbName     string
	adminUser  string
	pgedgeUser string
	sync       bool
	conn       *pgxpool.Pool // dst node's connection
	status     resource.Status
}

func NewSubscription(src, dst config.Node, dbName, adminUser, pgedgeUser string, sync bool, conn *pgxpool.Pool) *Subscription {
	return &Subscription{
		src:        src,
		dst:        dst,
		dbName:     dbName,
		adminUser:  adminUser,
		pgedgeUser: pgedgeUser,
		sync:       sync,
		conn:       conn,
	}
}

func (s *Subscription) subName() string {
	return strings.ReplaceAll(
		fmt.Sprintf("sub_%s_%s", s.src.Name, s.dst.Name),
		"-", "_",
	)
}

func (s *Subscription) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeSubscription, ID: s.subName()}
}

func (s *Subscription) replicationSlotID() string {
	return strings.ReplaceAll(
		fmt.Sprintf("spk_%s_%s_sub_%s_%s", s.dbName, s.src.Name, s.src.Name, s.dst.Name),
		"-", "_",
	)
}

func (s *Subscription) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeNode, ID: s.src.Name},
		{Type: ResourceTypeNode, ID: s.dst.Name},
		{Type: ResourceTypeReplicationSlot, ID: s.replicationSlotID()},
	}
}

func (s *Subscription) Refresh(ctx context.Context) error {
	var subExists bool
	err := s.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM spock.subscription WHERE sub_name = $1)",
		s.subName(),
	).Scan(&subExists)
	if err != nil {
		return fmt.Errorf("inspect subscription %s: %w", s.subName(), err)
	}

	if !subExists {
		s.status = resource.Status{Exists: false}
		return nil
	}

	s.status = resource.Status{Exists: true}
	return nil
}

func (s *Subscription) Status() resource.Status { return s.status }

func (s *Subscription) Create(ctx context.Context) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for subscription %s: %w", s.subName(), err)
	}
	defer tx.Rollback(ctx)

	dsn := fmt.Sprintf("host=%s dbname=%s user=%s %s port=5432",
		s.src.Hostname, s.dbName, s.pgedgeUser, sslSettings)

	_, err = tx.Exec(ctx, `SELECT spock.repair_mode('True')`)
	if err != nil {
		return fmt.Errorf("repair mode for subscription %s: %w", s.subName(), err)
	}

	_, err = tx.Exec(ctx, `
		SELECT spock.sub_create(
			subscription_name := $1,
			provider_dsn := $2,
			replication_sets := '{default, default_insert_only, ddl_sql}',
			synchronize_structure := $3,
			synchronize_data := $4,
			forward_origins := '{}',
			apply_delay := '0',
			enabled := 'true'
		)
		WHERE $1 NOT IN (SELECT sub_name FROM spock.subscription)
	`, s.subName(), dsn, s.sync, s.sync)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgCodeDuplicateObject {
			slog.Info("subscription already exists", "sub", s.subName())
			return nil
		}
		return fmt.Errorf("create subscription %s: %w", s.subName(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription %s: %w", s.subName(), err)
	}
	syncMsg := ""
	if s.sync {
		syncMsg = " (with initial sync)"
	}
	slog.Info("created subscription"+syncMsg, "sub", s.subName())
	return nil
}

func (s *Subscription) Delete(ctx context.Context) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx for %s: %w", s.subName(), err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `SELECT spock.repair_mode('True')`)
	if err != nil {
		return fmt.Errorf("repair mode for drop %s: %w", s.subName(), err)
	}

	_, err = tx.Exec(ctx, `SELECT spock.sub_drop($1, true)`, s.subName())
	if err != nil {
		return fmt.Errorf("drop subscription %s: %w", s.subName(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit drop subscription %s: %w", s.subName(), err)
	}
	slog.Info("dropped subscription", "sub", s.subName())
	return nil
}
