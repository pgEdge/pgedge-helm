// internal/spock/replication_slot_advance.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationSlotAdvanceFromCTS advances the provider-side replication slot
// to the LSN derived from the commit timestamp captured by the paired
// LagTrackerCommitTimestamp. Used during add-node populate to skip WAL the
// subscriber has already received via the source path, preventing
// double-apply when the peer→new subscription is enabled.
//
// AdvancedToLSN is set to the new LSN on a successful advance, empty when
// skipped. ReplicationOriginAdvance reads it to keep the subscriber-side
// origin in lockstep.
//
// Ephemeral resource — re-executes every run.
type ReplicationSlotAdvanceFromCTS struct {
	providerName   string // peer node (slot lives here)
	subscriberName string // new node
	dbName         string
	lagTracker     *LagTrackerCommitTimestamp
	conn           *pgxpool.Pool // peer's connection
	status         resource.Status

	// AdvancedToLSN records the LSN the slot was advanced to.
	// Empty when the advance was skipped (slot active, no commit_ts,
	// or target ≤ current). Read by ReplicationOriginAdvance to keep
	// the subscriber-side origin in lockstep with the provider-side slot.
	AdvancedToLSN string
}

func NewReplicationSlotAdvanceFromCTS(providerName, subscriberName, dbName string, lagTracker *LagTrackerCommitTimestamp, conn *pgxpool.Pool) *ReplicationSlotAdvanceFromCTS {
	return &ReplicationSlotAdvanceFromCTS{
		providerName:   providerName,
		subscriberName: subscriberName,
		dbName:         dbName,
		lagTracker:     lagTracker,
		conn:           conn,
	}
}

func (r *ReplicationSlotAdvanceFromCTS) slotName() string {
	return spockSlotName(r.dbName, r.providerName, r.subscriberName)
}

func (r *ReplicationSlotAdvanceFromCTS) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypeReplicationSlotAdvanceFromCTS,
		ID:   fmt.Sprintf("%s_%s", r.providerName, r.subscriberName),
	}
}

func (r *ReplicationSlotAdvanceFromCTS) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeNode, ID: r.providerName},
		{Type: ResourceTypeLagTrackerCommitTS, ID: fmt.Sprintf("%s_%s", r.providerName, r.subscriberName)},
	}
}

func (r *ReplicationSlotAdvanceFromCTS) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *ReplicationSlotAdvanceFromCTS) Status() resource.Status { return r.status }

func (r *ReplicationSlotAdvanceFromCTS) Create(ctx context.Context) error {
	// Reset before each invocation so the empty-when-skipped contract
	// holds for every skip path (no commit_ts, active slot, target ≤ current).
	r.AdvancedToLSN = ""

	slotName := r.slotName()

	if r.lagTracker == nil || r.lagTracker.CommitTS == nil || r.lagTracker.CommitTS.IsZero() {
		slog.Info("no commit timestamp available, skipping slot advance",
			"provider", r.providerName, "subscriber", r.subscriberName)
		return nil
	}

	// Convert commit_ts to target LSN
	var targetLSN string
	err := r.conn.QueryRow(ctx,
		"SELECT spock.get_lsn_from_commit_ts($1, $2)::text",
		slotName, *r.lagTracker.CommitTS,
	).Scan(&targetLSN)
	if err != nil {
		return fmt.Errorf("get target LSN from commit_ts for %s: %w", slotName, err)
	}

	// Skip if slot is actively used by a subscription — subscription is
	// already replicating and will advance the slot itself.
	var isActive bool
	err = r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1 AND active_pid IS NOT NULL)",
		slotName,
	).Scan(&isActive)
	if err != nil {
		return fmt.Errorf("check slot active %s: %w", slotName, err)
	}
	if isActive {
		slog.Info("slot is active, skipping advance", "slot", slotName)
		return nil
	}

	// Read current slot position. Skip if target is at or before current
	// (slots never go backwards, so this is safe without a transaction).
	// Compare via pg_lsn casts because LSN strings (e.g. "F/FFFFFFF" vs
	// "10/0") do not order correctly under Go string comparison.
	var currentLSN string
	err = r.conn.QueryRow(ctx,
		"SELECT restart_lsn::text FROM pg_replication_slots WHERE slot_name = $1",
		slotName,
	).Scan(&currentLSN)
	if err != nil {
		return fmt.Errorf("read current slot lsn %s: %w", slotName, err)
	}
	var atOrBefore bool
	err = r.conn.QueryRow(ctx,
		"SELECT $1::pg_lsn <= $2::pg_lsn",
		targetLSN, currentLSN,
	).Scan(&atOrBefore)
	if err != nil {
		return fmt.Errorf("compare slot lsn %s: %w", slotName, err)
	}
	if atOrBefore {
		slog.Info("slot already at or beyond target, no advance needed",
			"slot", slotName, "target", targetLSN, "current", currentLSN)
		return nil
	}

	// Advance the slot.
	_, err = r.conn.Exec(ctx, `
		WITH current AS (
			SELECT confirmed_flush_lsn
			FROM pg_replication_slots
			WHERE slot_name = $1
		)
		SELECT CASE
			WHEN $2::pg_lsn > confirmed_flush_lsn
			THEN (pg_replication_slot_advance($1, $2::pg_lsn)).end_lsn
			ELSE confirmed_flush_lsn
		END AS new_lsn
		FROM current`,
		slotName, targetLSN,
	)
	if err != nil {
		return fmt.Errorf("advance slot %s to %s: %w", slotName, targetLSN, err)
	}

	r.AdvancedToLSN = targetLSN
	slog.Info("advanced replication slot", "slot", slotName, "to", targetLSN)
	return nil
}

func (r *ReplicationSlotAdvanceFromCTS) Update(_ context.Context) error { return nil }

func (r *ReplicationSlotAdvanceFromCTS) Delete(_ context.Context) error { return nil }
