// internal/spock/replication_slot_create.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// ReplicationSlotCreate pre-creates a logical replication slot on a peer
// provider for a new subscriber. This is an ephemeral populate resource —
// Refresh always returns not-exists so it re-executes on every run.
type ReplicationSlotCreate struct {
	providerName   string
	subscriberName string
	dbName         string
	conn           *pgxpool.Pool
	status         resource.Status
}

func NewReplicationSlotCreate(providerName, subscriberName, dbName string, conn *pgxpool.Pool) *ReplicationSlotCreate {
	return &ReplicationSlotCreate{
		providerName:   providerName,
		subscriberName: subscriberName,
		dbName:         dbName,
		conn:           conn,
	}
}

func (r *ReplicationSlotCreate) slotName() string {
	return spockSlotName(r.dbName, r.providerName, r.subscriberName)
}

func (r *ReplicationSlotCreate) Identifier() resource.Identifier {
	return resource.Identifier{Type: ResourceTypeReplicationSlotCreate, ID: r.slotName()}
}

func (r *ReplicationSlotCreate) Dependencies() []resource.Identifier {
	return []resource.Identifier{{Type: ResourceTypeNode, ID: r.providerName}}
}

// Refresh always returns not-exists — ephemeral resource re-executes every run.
func (r *ReplicationSlotCreate) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *ReplicationSlotCreate) Status() resource.Status { return r.status }

func (r *ReplicationSlotCreate) Create(ctx context.Context) error {
	var exists bool
	err := r.conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)",
		r.slotName(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check slot %s: %w", r.slotName(), err)
	}
	if exists {
		slog.Info("replication slot already exists", "slot", r.slotName())
		return nil
	}

	_, err = r.conn.Exec(ctx,
		"SELECT pg_create_logical_replication_slot($1, 'spock_output')",
		r.slotName(),
	)
	if err != nil {
		return fmt.Errorf("create replication slot %s: %w", r.slotName(), err)
	}
	slog.Info("created replication slot", "slot", r.slotName())
	return nil
}

func (r *ReplicationSlotCreate) Update(_ context.Context) error { return nil }

// Delete is a no-op — the end-state ReplicationSlot resource manages slot lifecycle.
func (r *ReplicationSlotCreate) Delete(_ context.Context) error { return nil }
