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
}

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
	var ts time.Time
	err := r.conn.QueryRow(ctx,
		"SELECT commit_timestamp FROM spock.lag_tracker WHERE origin_name = $1 AND receiver_name = $2",
		r.originName, r.receiverName,
	).Scan(&ts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("no lag tracker entry", "origin", r.originName, "receiver", r.receiverName)
			r.CommitTS = nil
			return nil
		}
		return fmt.Errorf("query lag tracker %s→%s: %w", r.originName, r.receiverName, err)
	}

	r.CommitTS = &ts
	slog.Info("read lag tracker commit timestamp",
		"origin", r.originName, "receiver", r.receiverName, "commit_ts", ts)
	return nil
}

func (r *LagTrackerCommitTimestamp) Update(_ context.Context) error { return nil }

func (r *LagTrackerCommitTimestamp) Delete(_ context.Context) error { return nil }
