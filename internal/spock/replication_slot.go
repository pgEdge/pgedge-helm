// internal/spock/replication_slot.go
package spock

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationSlot manages a provider-side logical replication slot.
// Executes on the provider node's connection.
type ReplicationSlot struct {
	providerName   string
	subscriberName string
	dbName         string
	nameOverride   string // set for orphan slots discovered by raw name
	conn           *pgxpool.Pool
	status         resource.Status
}

func NewReplicationSlot(providerName, subscriberName, dbName string, conn *pgxpool.Pool) *ReplicationSlot {
	return &ReplicationSlot{
		providerName:   providerName,
		subscriberName: subscriberName,
		dbName:         dbName,
		conn:           conn,
	}
}

func (r *ReplicationSlot) slotName() string {
	if r.nameOverride != "" {
		return r.nameOverride
	}
	return strings.ReplaceAll(
		fmt.Sprintf("spk_%s_%s_sub_%s_%s", r.dbName, r.providerName, r.providerName, r.subscriberName),
		"-", "_",
	)
}

func (r *ReplicationSlot) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeReplicationSlot, ID: r.slotName()}
}

func (r *ReplicationSlot) Dependencies() []resource.Identifier {
	return []resource.Identifier{{Type: ResourceTypeNode, ID: r.providerName}}
}

func (r *ReplicationSlot) Refresh(ctx context.Context) error {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)",
		r.slotName(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check replication slot %s: %w", r.slotName(), err)
	}
	r.status = resource.Status{Exists: exists}
	return nil
}

func (r *ReplicationSlot) Status() resource.Status { return r.status }

// Create is a no-op — Spock's sub_create creates the slot automatically.
func (r *ReplicationSlot) Create(ctx context.Context) error {
	return nil
}

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
	slog.Info("dropped replication slot", "slot", r.slotName())
	return nil
}
