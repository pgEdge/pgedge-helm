// internal/spock/peer_catchup.go
package spock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

const peerCatchupPollInterval = 500 * time.Millisecond

// PeerCatchup waits until the source node's apply progress from the peer
// node has reached the peer's sync event LSN. This ensures the source→new
// COPY snapshot includes all peer writes up to the slot creation point,
// preventing data loss on add-node.
//
// Uses spock.progress.remote_lsn (apply progress at last committed
// transaction) rather than received_lsn, which can advance on keepalive
// messages before commits have been applied.
//
// Reads the target LSN from the paired SyncEvent (peer→source) via
// struct pointer.
//
// Ephemeral — re-executes every run.
type PeerCatchup struct {
	peerName   string
	sourceName string
	syncEvent  *SyncEvent
	conn       *pgxpool.Pool // source's connection
	status     resource.Status
}

func NewPeerCatchup(peerName, sourceName string, syncEvent *SyncEvent, conn *pgxpool.Pool) *PeerCatchup {
	return &PeerCatchup{
		peerName:   peerName,
		sourceName: sourceName,
		syncEvent:  syncEvent,
		conn:       conn,
	}
}

func (r *PeerCatchup) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypePeerCatchup,
		ID:   fmt.Sprintf("%s_%s", r.peerName, r.sourceName),
	}
}

func (r *PeerCatchup) Dependencies() []resource.Identifier {
	return []resource.Identifier{
		{Type: ResourceTypeSyncEvent, ID: fmt.Sprintf("%s_%s", r.peerName, r.sourceName)},
	}
}

func (r *PeerCatchup) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *PeerCatchup) Status() resource.Status { return r.status }

func (r *PeerCatchup) Create(ctx context.Context) error {
	if r.syncEvent == nil || r.syncEvent.LSN == "" {
		return fmt.Errorf("sync event LSN not available for peer catchup %s→%s",
			r.peerName, r.sourceName)
	}

	slog.Info("waiting for peer apply to catch up",
		"peer", r.peerName, "source", r.sourceName, "target_lsn", r.syncEvent.LSN)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var reached bool
		err := r.conn.QueryRow(ctx, `
			SELECT COALESCE(
				(SELECT p.remote_lsn >= $1::pg_lsn
				 FROM spock.progress p
				 JOIN spock.node n ON n.node_id = p.remote_node_id
				 WHERE p.node_id = (SELECT node_id FROM spock.node_info())
				   AND n.node_name = $2),
				false
			)`, r.syncEvent.LSN, r.peerName,
		).Scan(&reached)
		if err != nil {
			return fmt.Errorf("query spock.progress on %s for peer %s: %w",
				r.sourceName, r.peerName, err)
		}

		if reached {
			slog.Info("peer apply caught up",
				"peer", r.peerName, "source", r.sourceName, "target_lsn", r.syncEvent.LSN)
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(peerCatchupPollInterval):
		}
	}
}

func (r *PeerCatchup) Update(_ context.Context) error { return nil }

func (r *PeerCatchup) Delete(_ context.Context) error { return nil }
