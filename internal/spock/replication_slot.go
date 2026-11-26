// internal/spock/replication_slot.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationSlot manages a provider-side logical replication slot.
// Executes on the provider node's connection.
type ReplicationSlot struct {
	providerName   string
	subscriberName string
	name           string
	conn           *pgxpool.Pool
	status         resource.Status
}

func NewReplicationSlot(providerName, subscriberName, slotName string, conn *pgxpool.Pool) *ReplicationSlot {
	return &ReplicationSlot{
		providerName:   providerName,
		subscriberName: subscriberName,
		name:           slotName,
		conn:           conn,
	}
}

func (r *ReplicationSlot) slotName() string {
	return r.name
}

func (r *ReplicationSlot) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeReplicationSlot, ID: r.slotName()}
}

func (r *ReplicationSlot) Dependencies() []resource.Identifier {
	return []resource.Identifier{{Type: ResourceTypeNode, ID: r.providerName}}
}

func (r *ReplicationSlot) Refresh(ctx context.Context) error {
	// An invalidated slot (wal_status 'lost') stays in pg_replication_slots,
	// but can never be read from again, so its subscription stays down.
	var exists, lost bool
	err := r.conn.QueryRow(ctx,
		"SELECT count(*) > 0, coalesce(bool_or(wal_status = 'lost'), false) FROM pg_replication_slots WHERE slot_name = $1",
		r.slotName(),
	).Scan(&exists, &lost)
	if err != nil {
		return fmt.Errorf("check replication slot %s: %w", r.slotName(), err)
	}
	if lost {
		r.status = resource.Status{Exists: true, NeedsRecreate: true, Reason: "replication slot is invalidated"}
		return nil
	}
	r.status = resource.Status{Exists: exists}
	return nil
}

func (r *ReplicationSlot) Status() resource.Status { return r.status }

// Create is a no-op — Spock's sub_create creates the slot automatically.
func (r *ReplicationSlot) Create(ctx context.Context) error {
	return nil
}

func (r *ReplicationSlot) Update(_ context.Context) error { return nil }

// Delete terminates any active walsender and drops the replication slot.
func (r *ReplicationSlot) Delete(ctx context.Context) error {
	tx, err := r.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx for slot %s: %w", r.slotName(), err)
	}
	defer tx.Rollback(ctx)

	// Terminate active walsender — pg_drop_replication_slot fails on active slots.
	_, err = tx.Exec(ctx, `
		SELECT pg_terminate_backend(active_pid)
		  FROM pg_replication_slots
		 WHERE slot_name = $1 AND active`, r.slotName())
	if err != nil {
		return fmt.Errorf("terminate walsender for slot %s: %w", r.slotName(), err)
	}

	// Drop the slot if it still exists.
	var slotExists bool
	err = tx.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)",
		r.slotName(),
	).Scan(&slotExists)
	if err != nil {
		return fmt.Errorf("check slot %s before drop: %w", r.slotName(), err)
	}
	if slotExists {
		_, err = tx.Exec(ctx, "SELECT pg_drop_replication_slot($1)", r.slotName())
		if err != nil {
			return fmt.Errorf("drop replication slot %s: %w", r.slotName(), err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit drop slot %s: %w", r.slotName(), err)
	}
	if slotExists {
		slog.Info("dropped replication slot", "slot", r.slotName())
	}
	return nil
}
