// internal/spock/sync_event.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// SyncEvent inserts a WAL bookmark on a provider node via spock.sync_event().
// The returned LSN is stored for the paired WaitForSyncEvent to read.
// Ephemeral resource — re-executes every run.
type SyncEvent struct {
	providerName   string
	subscriberName string
	conn           *pgxpool.Pool
	extraDeps      []resource.Identifier
	status         resource.Status
	LSN            string // populated during Create
}

func NewSyncEvent(providerName, subscriberName string, conn *pgxpool.Pool, extraDeps ...resource.Identifier) *SyncEvent {
	return &SyncEvent{
		providerName:   providerName,
		subscriberName: subscriberName,
		conn:           conn,
		extraDeps:      extraDeps,
	}
}

func (r *SyncEvent) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypeSyncEvent,
		ID:   fmt.Sprintf("%s_%s", r.providerName, r.subscriberName),
	}
}

func (r *SyncEvent) Dependencies() []resource.Identifier {
	deps := []resource.Identifier{
		{Type: ResourceTypeNode, ID: r.providerName},
		{Type: ResourceTypeSubscription, ID: fmt.Sprintf("sub_%s_%s", r.providerName, r.subscriberName)},
	}
	return append(deps, r.extraDeps...)
}

func (r *SyncEvent) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *SyncEvent) Status() resource.Status { return r.status }

func (r *SyncEvent) Create(ctx context.Context) error {
	tx, err := r.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for sync event %s→%s: %w", r.providerName, r.subscriberName, err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT spock.repair_mode('True')")
	if err != nil {
		return fmt.Errorf("repair mode for sync event: %w", err)
	}

	err = tx.QueryRow(ctx, "SELECT spock.sync_event()").Scan(&r.LSN)
	if err != nil {
		return fmt.Errorf("sync event on %s: %w", r.providerName, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit sync event: %w", err)
	}

	slog.Info("sent sync event", "provider", r.providerName, "subscriber", r.subscriberName, "lsn", r.LSN)
	return nil
}

func (r *SyncEvent) Update(_ context.Context) error { return nil }

func (r *SyncEvent) Delete(_ context.Context) error { return nil }
