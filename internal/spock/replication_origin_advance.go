// internal/spock/replication_origin_advance.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationOriginAdvance advances pg_replication_origin on the subscriber
// to the LSN ReplicationSlotAdvanceFromCTS just advanced the provider-side
// slot to. The slot (provider) and origin (subscriber) must agree to prevent
// the apply worker from replaying historical WAL from 0/0.
//
// Separate from ReplicationSlotAdvanceFromCTS because the slot lives on the
// provider and the origin on the subscriber — they use different connections.
//
// Reads the target LSN from the paired ReplicationSlotAdvanceFromCTS via
// struct pointer. No-ops when AdvancedToLSN is empty (slot advance was
// skipped because slot active, no commit_ts, or already at target).
//
// Ephemeral — re-executes every run.
type ReplicationOriginAdvance struct {
	providerName   string
	subscriberName string
	name           string
	slotAdvance    *ReplicationSlotAdvanceFromCTS
	conn           *pgxpool.Pool // subscriber's connection
	status         resource.Status
}

func NewReplicationOriginAdvance(providerName, subscriberName, originName string, slotAdvance *ReplicationSlotAdvanceFromCTS, conn *pgxpool.Pool) *ReplicationOriginAdvance {
	return &ReplicationOriginAdvance{
		providerName:   providerName,
		subscriberName: subscriberName,
		name:           originName,
		slotAdvance:    slotAdvance,
		conn:           conn,
	}
}

// originName returns the replication origin name. Spock gives the origin the
// same name as the provider-side slot.
func (r *ReplicationOriginAdvance) originName() string {
	return r.name
}

func (r *ReplicationOriginAdvance) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypeReplicationOriginAdvance,
		ID:   fmt.Sprintf("%s_%s", r.providerName, r.subscriberName),
	}
}

func (r *ReplicationOriginAdvance) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeReplicationSlotAdvanceFromCTS, ID: fmt.Sprintf("%s_%s", r.providerName, r.subscriberName)},
	}
}

func (r *ReplicationOriginAdvance) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *ReplicationOriginAdvance) Status() resource.Status { return r.status }

func (r *ReplicationOriginAdvance) Create(ctx context.Context) error {
	if r.slotAdvance == nil || r.slotAdvance.AdvancedToLSN == "" {
		slog.Info("slot advance was skipped, no origin to advance",
			"provider", r.providerName, "subscriber", r.subscriberName)
		return nil
	}

	originName := r.originName()
	targetLSN := r.slotAdvance.AdvancedToLSN

	// Create the origin if it doesn't already exist. spock.sub_create
	// usually creates it, but the NOT EXISTS guard makes this safe in
	// any ordering. Matches upstream's EnsureReplicationOriginExists.
	_, err := r.conn.Exec(ctx, `
		SELECT pg_replication_origin_create($1)
		WHERE NOT EXISTS (
			SELECT 1 FROM pg_replication_origin WHERE roname = $1
		)`, originName)
	if err != nil {
		return fmt.Errorf("ensure replication origin %s: %w", originName, err)
	}

	_, err = r.conn.Exec(ctx,
		"SELECT pg_replication_origin_advance($1, $2::pg_lsn)",
		originName, targetLSN,
	)
	if err != nil {
		return fmt.Errorf("advance replication origin %s to %s: %w", originName, targetLSN, err)
	}

	slog.Info("advanced replication origin", "origin", originName, "to", targetLSN)
	return nil
}

func (r *ReplicationOriginAdvance) Update(_ context.Context) error { return nil }

func (r *ReplicationOriginAdvance) Delete(_ context.Context) error { return nil }
