// internal/spock/lag_tracker_commit_ts.go
package spock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// LagTrackerCommitTimestamp reads the commit timestamp from spock.lag_tracker
// on the new node. Captures how far peer data has been replicated.
// Ephemeral resource — re-executes every run.
type LagTrackerCommitTimestamp struct {
	originName   string // peer node
	receiverName string // new node
	conn         *pgxpool.Pool
	extraDeps    []resource.Identifier
	status       resource.Status
	CommitTS     *time.Time // populated during Create
	// CommitLSN is the origin's LSN of the last commit from originName that
	// the new node holds (spock.lag_tracker.commit_lsn, Spock 6.0 and
	// later). Empty when unknown, and always on Spock 5.0.x, whose
	// lag_tracker has no such column.
	CommitLSN string
}

// lagTrackerQuery reads the commit timestamp and, on Spock 6.0 and later,
// the commit LSN. to_jsonb keeps the query valid on Spock 5.0.x, where
// lag_tracker has no commit_lsn column, by returning NULL for it.
const lagTrackerQuery = `
	SELECT lt.commit_timestamp, to_jsonb(lt)->>'commit_lsn'
	FROM spock.lag_tracker lt
	WHERE lt.origin_name = $1 AND lt.receiver_name = $2`

func NewLagTrackerCommitTimestamp(originName, receiverName string, conn *pgxpool.Pool, extraDeps ...resource.Identifier) *LagTrackerCommitTimestamp {
	return &LagTrackerCommitTimestamp{
		originName:   originName,
		receiverName: receiverName,
		conn:         conn,
		extraDeps:    extraDeps,
	}
}

func (r *LagTrackerCommitTimestamp) Identifier() resource.Identifier {
	return resource.Identifier{
		Type: ResourceTypeLagTrackerCommitTS,
		ID:   fmt.Sprintf("%s_%s", r.originName, r.receiverName),
	}
}

func (r *LagTrackerCommitTimestamp) Dependencies() []resource.Identifier {
	deps := []resource.Identifier{
		{Type: ResourceTypeNode, ID: r.receiverName},
	}
	return append(deps, r.extraDeps...)
}

func (r *LagTrackerCommitTimestamp) Refresh(_ context.Context) error {
	r.status = resource.Status{Exists: false}
	return nil
}

func (r *LagTrackerCommitTimestamp) Status() resource.Status { return r.status }

func (r *LagTrackerCommitTimestamp) Create(ctx context.Context) error {
	r.CommitTS = nil
	r.CommitLSN = ""

	// Either value can be NULL: on Spock 6.0, spock.lag_tracker is a view
	// over spock.progress that returns a row for a subscription as soon as
	// it exists, and Spock clears the timestamp when it reconciles progress
	// after a restart or sync, until it next applies a commit from that
	// origin. The commit LSN is kept.
	var ts *time.Time
	var lsn *string
	err := r.conn.QueryRow(ctx, lagTrackerQuery, r.originName, r.receiverName).Scan(&ts, &lsn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("no lag tracker entry", "origin", r.originName, "receiver", r.receiverName)
			return nil
		}
		return fmt.Errorf("query lag tracker %s→%s: %w", r.originName, r.receiverName, err)
	}

	if ts != nil && !ts.IsZero() {
		r.CommitTS = ts
	}
	if lsn != nil {
		r.CommitLSN = *lsn
	}
	if r.CommitTS == nil && r.CommitLSN == "" {
		slog.Warn("lag tracker entry has no commit timestamp or LSN",
			"origin", r.originName, "receiver", r.receiverName)
		return nil
	}
	slog.Info("read lag tracker", "origin", r.originName, "receiver", r.receiverName,
		"commit_ts", r.CommitTS, "commit_lsn", r.CommitLSN)
	return nil
}

func (r *LagTrackerCommitTimestamp) Update(_ context.Context) error { return nil }

func (r *LagTrackerCommitTimestamp) Delete(_ context.Context) error { return nil }
