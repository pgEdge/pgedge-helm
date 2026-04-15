// internal/spock/disabled_subscription.go
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

// DisabledSubscription creates a disabled subscription from a peer provider to
// the new node during populate. This registers the subscription in Spock metadata
// early so the lag tracker can populate as data flows. The end-state Subscription
// resource later detects the disabled state and enables it via Update.
// Ephemeral resource — re-executes every run.
type DisabledSubscription struct {
	src        config.Node
	dst        config.Node
	dbName     string
	pgedgeUser string
	conn       *pgxpool.Pool // dst node's connection
	status     resource.Status
}

func NewDisabledSubscription(src, dst config.Node, dbName, pgedgeUser string, conn *pgxpool.Pool) *DisabledSubscription {
	return &DisabledSubscription{
		src:        src,
		dst:        dst,
		dbName:     dbName,
		pgedgeUser: pgedgeUser,
		conn:       conn,
	}
}

func (s *DisabledSubscription) subName() string {
	return strings.ReplaceAll(
		fmt.Sprintf("sub_%s_%s", s.src.Name, s.dst.Name),
		"-", "_",
	)
}

func (s *DisabledSubscription) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeDisabledSubscription, ID: s.subName()}
}

func (s *DisabledSubscription) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeNode, ID: s.src.Name},
		{Type: ResourceTypeNode, ID: s.dst.Name},
		{Type: ResourceTypeReplicationSlotCreate, ID: strings.ReplaceAll(
			fmt.Sprintf("spk_%s_%s_sub_%s_%s", s.dbName, s.src.Name, s.src.Name, s.dst.Name),
			"-", "_",
		)},
	}
}

// Refresh always returns not-exists — ephemeral resource re-executes every run.
func (s *DisabledSubscription) Refresh(_ context.Context) error {
	s.status = resource.Status{Exists: false}
	return nil
}

func (s *DisabledSubscription) Status() resource.Status { return s.status }

func (s *DisabledSubscription) Create(ctx context.Context) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for disabled subscription %s: %w", s.subName(), err)
	}
	defer tx.Rollback(ctx)

	dsn := fmt.Sprintf("host=%s dbname=%s user=%s %s port=5432",
		s.src.Hostname, s.dbName, s.pgedgeUser, sslSettings)

	_, err = tx.Exec(ctx, `SELECT spock.repair_mode('True')`)
	if err != nil {
		return fmt.Errorf("repair mode for disabled subscription %s: %w", s.subName(), err)
	}

	_, err = tx.Exec(ctx, `
		SELECT spock.sub_create(
			subscription_name := $1,
			provider_dsn := $2,
			replication_sets := '{default, default_insert_only, ddl_sql}',
			synchronize_structure := false,
			synchronize_data := false,
			forward_origins := '{}',
			apply_delay := '0',
			enabled := 'false'
		)
		WHERE $1 NOT IN (SELECT sub_name FROM spock.subscription)
	`, s.subName(), dsn)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42710" {
			slog.Info("disabled subscription already exists", "sub", s.subName())
		} else {
			return fmt.Errorf("create disabled subscription %s: %w", s.subName(), err)
		}
	} else {
		slog.Info("created disabled subscription", "sub", s.subName())
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit disabled subscription %s: %w", s.subName(), err)
	}
	return nil
}

func (s *DisabledSubscription) Update(_ context.Context) error { return nil }

func (s *DisabledSubscription) Delete(_ context.Context) error { return nil }
