// internal/spock/wait_for_sync_event.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

const (
	syncEventTimeout = 10 // seconds, passed to spock.wait_for_sync_event
)

// WaitForSyncEvent polls on the subscriber until it receives the sync event
// from the provider. Reads the LSN from the paired SyncEvent via struct pointer.
// Ephemeral resource — re-executes every run.
type WaitForSyncEvent struct {
	providerName   string
	subscriberName string
	syncEvent      *SyncEvent
	conn           *pgxpool.Pool // subscriber's connection
	status         resource.Status
}

func NewWaitForSyncEvent(providerName, subscriberName string, syncEvent *SyncEvent, conn *pgxpool.Pool) *WaitForSyncEvent {
	return &WaitForSyncEvent{
		providerName:   providerName,
		subscriberName: subscriberName,
		syncEvent:      syncEvent,
		conn:           conn,
	}
}

func (r *WaitForSyncEvent) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypeWaitForSyncEvent,
		ID:   fmt.Sprintf("%s_%s", r.providerName, r.subscriberName),
	}
}

func (r *WaitForSyncEvent) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeSyncEvent, ID: fmt.Sprintf("%s_%s", r.providerName, r.subscriberName)},
	}
}

func (r *WaitForSyncEvent) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *WaitForSyncEvent) Status() resource.Status { return r.status }

func (r *WaitForSyncEvent) Create(ctx context.Context) error {
	if r.syncEvent == nil || r.syncEvent.LSN == "" {
		return fmt.Errorf("sync event LSN not available for %s→%s", r.providerName, r.subscriberName)
	}

	slog.Info("waiting for sync event",
		"provider", r.providerName, "subscriber", r.subscriberName, "lsn", r.syncEvent.LSN)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Check subscription health — fail early if broken
		subName := spockSubName(r.providerName, r.subscriberName)
		var status string
		err := r.conn.QueryRow(ctx,
			"SELECT status FROM spock.sub_show_status() WHERE subscription_name = $1",
			subName,
		).Scan(&status)
		if err != nil {
			slog.Warn("failed to check subscription status", "error", err)
			// Don't fail — status query might not work during initialization
		} else {
			switch status {
			case "disabled", "down":
				return fmt.Errorf("subscription has unhealthy status %q: provider=%s subscriber=%s",
					status, r.providerName, r.subscriberName)
			}
		}

		// Wait for sync event — CALL returns the INOUT synced boolean.
		// The procedure blocks for up to $timeout seconds.
		var synced bool
		err = r.conn.QueryRow(ctx,
			"CALL spock.wait_for_sync_event(true, $1, $2, $3)",
			r.providerName, r.syncEvent.LSN, syncEventTimeout,
		).Scan(&synced)
		if err != nil {
			return fmt.Errorf("wait_for_sync_event for %s→%s: %w",
				r.providerName, r.subscriberName, err)
		}
		if synced {
			slog.Info("sync event confirmed",
				"provider", r.providerName, "subscriber", r.subscriberName)
			return nil
		}

		// Not yet synced but subscription is healthy — continue polling
	}
}

func (r *WaitForSyncEvent) Update(_ context.Context) error { return nil }

func (r *WaitForSyncEvent) Delete(_ context.Context) error { return nil }
