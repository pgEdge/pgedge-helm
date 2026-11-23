// internal/spock/wait_for_sync_event.go
package spock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

const (
	syncEventTimeout = 10 // seconds, passed to spock.wait_for_sync_event

	initialSyncPollInterval = 1 * time.Second
)

// pendingInitialSyncQuery counts the subscription's sync entries that are
// not done yet ('y' = sync done, 'r' = ready), the same check Spock's own
// add-node procedure (Zodan) uses before it reads progress or advances
// slots. Works on Spock 5.0.x and 6.0.
const pendingInitialSyncQuery = `
	SELECT count(*)
	FROM spock.local_sync_status ls
	JOIN spock.subscription s ON s.sub_id = ls.sync_subid
	WHERE s.sub_name = $1 AND ls.sync_status NOT IN ('y', 'r')`

// WaitForSyncEvent polls on the subscriber until it receives the sync event
// from the provider. Reads the LSN from the paired SyncEvent via struct pointer.
//
// Transient subscription states (disabled, down) trigger backoff polling
// rather than immediate failure — the apply worker may not have started yet
// during add-node. Unknown subscription states are treated as errors
// (fail-closed default).
//
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
			case "initializing", "replicating", "unknown":
				// Worker is running — fall through to wait_for_sync_event call.
			case "disabled", "down":
				// Worker not yet started — transient during add-node. Sleep
				// before next iteration to avoid busy-looping while the
				// worker comes up.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(syncEventTimeout * time.Second):
				}
				continue
			default:
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
			return r.waitForInitialSync(ctx, subName)
		}

		// Not yet synced but subscription is healthy — continue polling
	}
}

// waitForInitialSync waits until the subscription's initial structure and
// data sync has finished. On Spock 6.0, wait_for_sync_event compares the
// event's LSN with the subscription's replication origin, which Spock sets
// to the snapshot LSN before the copy runs, so the event can be confirmed
// while a new node is still being populated. Everything that depends on
// this wait (reading lag tracker progress, advancing peer slots, enabling
// the peers' subscriptions to the new node) must happen after the copy,
// otherwise peers' changes can reach the new node first and break the
// restore. On Spock 5.0.x the sync has already finished at this point.
func (r *WaitForSyncEvent) waitForInitialSync(ctx context.Context, subName string) error {
	logged := false
	for {
		var pending int
		if err := r.conn.QueryRow(ctx, pendingInitialSyncQuery, subName).Scan(&pending); err != nil {
			return fmt.Errorf("check initial sync of %s: %w", subName, err)
		}
		if pending == 0 {
			if logged {
				slog.Info("initial sync finished", "sub", subName)
			}
			return nil
		}

		var status string
		if err := r.conn.QueryRow(ctx,
			"SELECT status FROM spock.sub_show_status() WHERE subscription_name = $1", subName,
		).Scan(&status); err == nil && status == "disabled" {
			return fmt.Errorf("initial sync of subscription %s (%s→%s) failed: Spock disabled the "+
				"subscription; check the PostgreSQL log on %s for the cause",
				subName, r.providerName, r.subscriberName, r.subscriberName)
		}

		if !logged {
			slog.Info("waiting for initial sync to finish", "sub", subName, "pending", pending)
			logged = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initialSyncPollInterval):
		}
	}
}

func (r *WaitForSyncEvent) Update(_ context.Context) error { return nil }

func (r *WaitForSyncEvent) Delete(_ context.Context) error { return nil }
