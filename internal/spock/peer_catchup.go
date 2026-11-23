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

// peerCatchupQuery reports whether the local node has applied everything from
// the named peer up to the given LSN.
//
// It reads pg_replication_origin_status.remote_lsn for the origin of the
// peer→local subscription (Spock names the origin after the subscription's
// slot), which is the end LSN of the last remote transaction applied and
// committed locally. This works the same on Spock 5.0.x and 6.0, and is what
// Spock's own wait_for_sync_event() checks since 5.0.7.
//
// It does not read spock.progress: on 5.0.x that is a table with a remote_lsn
// column, but on 6.0 it is a view over spock.apply_group_progress() with no
// remote_lsn (the closest column is remote_commit_lsn), so a query against
// it can't be written for both versions.
//
// Returns false, rather than no row, while the subscription or its origin
// doesn't exist yet.
const peerCatchupQuery = `
	SELECT COALESCE(
		(SELECT ros.remote_lsn >= $1::pg_lsn
		 FROM spock.subscription s
		 JOIN spock.node n ON n.node_id = s.sub_origin
		 JOIN pg_replication_origin_status ros ON ros.external_id = s.sub_slot_name
		 WHERE s.sub_target = (SELECT node_id FROM spock.node_info())
		   AND n.node_name = $2),
		false
	)`

// PeerCatchup waits until the source node's apply progress from the peer
// node has reached the peer's sync event LSN. This ensures the source→new
// COPY snapshot includes all peer writes up to the slot creation point,
// preventing data loss on add-node.
//
// Reads apply progress from pg_replication_origin_status.remote_lsn for the
// peer→source subscription's origin (named after its slot). That is the end
// LSN of the last remote transaction the source has committed, so it doesn't
// move on keepalives the way received_lsn does. See peerCatchupQuery for why
// this source is used instead of spock.progress.
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
		err := r.conn.QueryRow(ctx, peerCatchupQuery, r.syncEvent.LSN, r.peerName).Scan(&reached)
		if err != nil {
			return fmt.Errorf("query apply progress on %s for peer %s: %w",
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
