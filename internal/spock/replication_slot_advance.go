// internal/spock/replication_slot_advance.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationSlotAdvanceFromCTS advances a peer's replication slot to skip
// WAL already synced via the source. Reads the commit timestamp from the
// paired LagTrackerCommitTimestamp via struct pointer.
// Ephemeral resource — re-executes every run.
type ReplicationSlotAdvanceFromCTS struct {
	providerName   string // peer node (slot lives here)
	subscriberName string // new node
	dbName         string
	lagTracker     *LagTrackerCommitTimestamp
	conn           *pgxpool.Pool // peer's connection
	status         resource.Status
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

	// Check if slot is actively used by a subscription — skip if yes
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

	// Compare and advance atomically in a single query, matching the
	// control-plane's CTE pattern. This handles NULL confirmed_flush_lsn
	// on freshly created slots and avoids a TOCTOU race.
	var newLSN string
	err = r.conn.QueryRow(ctx, `
		WITH current AS (
			SELECT confirmed_flush_lsn
			FROM pg_replication_slots
			WHERE slot_name = $1
		)
		SELECT CASE
			WHEN $2::pg_lsn > COALESCE(confirmed_flush_lsn, '0/0'::pg_lsn)
			THEN (pg_replication_slot_advance($1, $2::pg_lsn)).end_lsn
			ELSE confirmed_flush_lsn
		END AS new_lsn
		FROM current`,
		slotName, targetLSN,
	).Scan(&newLSN)
	if err != nil {
		return fmt.Errorf("advance slot %s to %s: %w", slotName, targetLSN, err)
	}

	slog.Info("advanced replication slot", "slot", slotName, "to", newLSN)
	return nil
}

func (r *ReplicationSlotAdvanceFromCTS) Update(_ context.Context) error { return nil }

func (r *ReplicationSlotAdvanceFromCTS) Delete(_ context.Context) error { return nil }
