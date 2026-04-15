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
	pgedgeUser string
	sync       bool
	conn       *pgxpool.Pool // dst node's connection
	status     resource.Status
	extraDeps  []resource.Identifier
}

func NewSubscription(src, dst config.Node, dbName, pgedgeUser string, sync bool, conn *pgxpool.Pool, extraDeps ...resource.Identifier) *Subscription {
	return &Subscription{
		src:        src,
		dst:        dst,
		dbName:     dbName,
		pgedgeUser: pgedgeUser,
		sync:       sync,
		conn:       conn,
		extraDeps:  extraDeps,
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

func (s *Subscription) replicationSlotName() string {
	return strings.ReplaceAll(
		fmt.Sprintf("spk_%s_%s_sub_%s_%s", s.dbName, s.src.Name, s.src.Name, s.dst.Name),
		"-", "_",
	)
}

func (s *Subscription) Dependencies() []resource.Identifier {
	deps := []resource.Identifier{
		{Type: ResourceTypeNode, ID: s.src.Name},
		{Type: ResourceTypeNode, ID: s.dst.Name},
		{Type: ResourceTypeReplicationSlot, ID: s.replicationSlotName()},
	}
	return append(deps, s.extraDeps...)
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

	// A disabled subscription that should be enabled triggers an update.
	var isEnabled bool
	err = s.conn.QueryRow(ctx,
		"SELECT sub_enabled FROM spock.subscription WHERE sub_name = $1",
		s.subName(),
	).Scan(&isEnabled)
	if err != nil {
		return fmt.Errorf("check subscription enabled %s: %w", s.subName(), err)
	}
	if !isEnabled {
		s.status = resource.Status{Exists: true, NeedsUpdate: true, Reason: "subscription is disabled"}
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

	// Check if the subscription already exists (e.g. created disabled by populate).
	// If it exists and we want it enabled, enable it instead of recreating.
	var existsDisabled bool
	err = tx.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM spock.subscription WHERE sub_name = $1 AND NOT sub_enabled)",
		s.subName(),
	).Scan(&existsDisabled)
	if err != nil {
		return fmt.Errorf("check disabled subscription %s: %w", s.subName(), err)
	}

	if existsDisabled {
		_, err = tx.Exec(ctx, `SELECT spock.sub_enable($1)`, s.subName())
		if err != nil {
			return fmt.Errorf("enable subscription %s: %w", s.subName(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit enable subscription %s: %w", s.subName(), err)
		}
		slog.Info("enabled existing disabled subscription", "sub", s.subName())
		return nil
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
	msg := "created subscription"
	if s.sync {
		msg += " (with initial sync)"
	}
	slog.Info(msg, "sub", s.subName())
	return nil
}

func (s *Subscription) Update(ctx context.Context) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for subscription update %s: %w", s.subName(), err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `SELECT spock.repair_mode('True')`)
	if err != nil {
		return fmt.Errorf("repair mode for subscription update %s: %w", s.subName(), err)
	}

	_, err = tx.Exec(ctx, `SELECT spock.sub_enable($1)`, s.subName())
	if err != nil {
		return fmt.Errorf("enable subscription %s: %w", s.subName(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription update %s: %w", s.subName(), err)
	}
	slog.Info("enabled subscription", "sub", s.subName())
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
