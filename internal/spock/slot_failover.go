// internal/spock/slot_failover.go
package spock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

const (
	slotReleasePollInterval = 1 * time.Second
	// slotReleaseTimeout bounds the wait for a walsender to let go of a slot
	// after its subscription is disabled. The chart's wal_sender_timeout is
	// 5s, so this is generous.
	slotReleaseTimeout = 60 * time.Second
)

// slotFailoverCapableQuery reports whether the node can mark Spock slots for
// failover: PostgreSQL 17 or later, and Spock 6.0 or later, which provides
// spock.slot_enable_failover(). The function marks slots in place, which
// plain SQL can't do for an existing slot.
const slotFailoverCapableQuery = `
	SELECT current_setting('server_version_num')::int >= 170000
	   AND to_regprocedure('spock.slot_enable_failover()') IS NOT NULL`

// slotsWithoutFailoverQuery lists the node's own Spock slots that don't have
// the failover flag. Copies synced from a primary, temporary slots and
// invalidated slots can't take the flag, so they are left out.
const slotsWithoutFailoverQuery = `
	SELECT slot_name, active
	FROM pg_replication_slots
	WHERE plugin = 'spock_output'
	  AND NOT temporary
	  AND NOT failover
	  AND NOT synced
	  AND invalidation_reason IS NULL
	ORDER BY slot_name`

type slotState struct {
	name   string
	active bool
}

// EnableSlotFailover makes sure every Spock slot on PostgreSQL 17 and later
// carries the failover flag, so PostgreSQL's native slot sync copies it to
// physical standbys. On PostgreSQL 18 with Spock 6.0 that is the only way a
// slot survives a failover.
//
// Slots created by Spock 6.0 (and by this job) already have it. Slots
// created by Spock 5.0.x get it from the 5.0.14 → 6.0.0 extension update,
// except for slots a walsender held at that moment, which is likely right
// after a rolling image change. For each of those, this disables the
// subscription that reads the slot, waits for the walsender to let go,
// marks the slot, and enables the subscription again. Disabling a
// subscription only pauses apply: the slot keeps its position, so nothing is
// lost or replayed.
//
// Nodes on PostgreSQL 16 or older, or on Spock 5.0.x, are skipped.
func EnableSlotFailover(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	for _, provider := range cfg.Nodes {
		if err := enableSlotFailoverOn(ctx, cfg, conns, provider.Name); err != nil {
			return err
		}
	}
	return nil
}

func enableSlotFailoverOn(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool, providerName string) error {
	conn := conns[providerName]

	var capable bool
	if err := conn.QueryRow(ctx, slotFailoverCapableQuery).Scan(&capable); err != nil {
		return fmt.Errorf("check slot failover support on %s: %w", providerName, err)
	}
	if !capable {
		return nil
	}

	slots, err := slotsWithoutFailover(ctx, conn)
	if err != nil {
		return fmt.Errorf("list spock slots without failover on %s: %w", providerName, err)
	}
	if len(slots) == 0 {
		return nil
	}

	// Mark every idle slot in one go.
	if _, err := conn.Exec(ctx, "SELECT spock.slot_enable_failover()"); err != nil {
		return fmt.Errorf("enable slot failover on %s: %w", providerName, err)
	}

	slots, err = slotsWithoutFailover(ctx, conn)
	if err != nil {
		return fmt.Errorf("list spock slots without failover on %s: %w", providerName, err)
	}
	for _, slot := range slots {
		if err := enableFailoverForBusySlot(ctx, cfg, conns, providerName, slot.name); err != nil {
			return err
		}
	}
	return nil
}

func slotsWithoutFailover(ctx context.Context, conn *pgxpool.Pool) ([]slotState, error) {
	rows, err := conn.Query(ctx, slotsWithoutFailoverQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []slotState
	for rows.Next() {
		var s slotState
		if err := rows.Scan(&s.name, &s.active); err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

// enableFailoverForBusySlot pauses the subscription reading slotName so the
// slot is free, marks it for failover, and resumes the subscription.
func enableFailoverForBusySlot(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool, providerName, slotName string) error {
	subscriberName, subName, err := findSubscriptionForSlot(ctx, cfg, conns, providerName, slotName)
	if err != nil {
		return err
	}
	if subName == "" {
		// Not read by a node this job manages: nothing safe to pause.
		slog.Warn("spock slot has no failover flag and no known subscription reads it; "+
			"it will not be copied to standbys until spock.slot_enable_failover() is run while it is idle",
			"node", providerName, "slot", slotName)
		return nil
	}
	subConn := conns[subscriberName]

	slog.Info("pausing subscription to enable failover on its slot",
		"provider", providerName, "subscriber", subscriberName, "sub", subName, "slot", slotName)
	if err := setSubscriptionEnabled(ctx, subConn, subName, false); err != nil {
		return fmt.Errorf("disable subscription %s on %s: %w", subName, subscriberName, err)
	}
	// Re-enable even if a later step fails or the job is cancelled.
	defer func() {
		enableCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := setSubscriptionEnabled(enableCtx, subConn, subName, true); err != nil {
			slog.Error("failed to re-enable subscription; enable it with spock.sub_enable()",
				"subscriber", subscriberName, "sub", subName, "error", err)
			return
		}
		slog.Info("resumed subscription", "subscriber", subscriberName, "sub", subName)
	}()

	if err := waitForSlotReleased(ctx, conns[providerName], slotName); err != nil {
		return fmt.Errorf("wait for slot %s on %s to be released: %w", slotName, providerName, err)
	}

	if _, err := conns[providerName].Exec(ctx, "SELECT spock.slot_enable_failover()"); err != nil {
		return fmt.Errorf("enable slot failover on %s: %w", providerName, err)
	}

	var failover bool
	err = conns[providerName].QueryRow(ctx,
		"SELECT failover FROM pg_replication_slots WHERE slot_name = $1", slotName,
	).Scan(&failover)
	if err != nil {
		return fmt.Errorf("check failover flag of slot %s on %s: %w", slotName, providerName, err)
	}
	if !failover {
		slog.Warn("spock slot still has no failover flag", "node", providerName, "slot", slotName)
		return nil
	}
	slog.Info("enabled failover on spock slot", "node", providerName, "slot", slotName)
	return nil
}

// findSubscriptionForSlot returns the node and subscription that read
// slotName on providerName, or empty strings if no managed node does.
func findSubscriptionForSlot(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool, providerName, slotName string) (string, string, error) {
	for _, node := range cfg.Nodes {
		if node.Name == providerName {
			continue
		}
		var subName string
		err := conns[node.Name].QueryRow(ctx, `
			SELECT COALESCE((
				SELECT s.sub_name::text
				FROM spock.subscription s
				JOIN spock.node n ON n.node_id = s.sub_origin
				WHERE s.sub_slot_name = $1 AND n.node_name = $2), '')`,
			slotName, providerName,
		).Scan(&subName)
		if err != nil {
			return "", "", fmt.Errorf("look up subscription for slot %s on %s: %w", slotName, node.Name, err)
		}
		if subName != "" {
			return node.Name, subName, nil
		}
	}
	return "", "", nil
}

func setSubscriptionEnabled(ctx context.Context, conn *pgxpool.Pool, subName string, enabled bool) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT spock.repair_mode('True')`); err != nil {
		return err
	}
	fn := "spock.sub_disable"
	if enabled {
		fn = "spock.sub_enable"
	}
	if _, err := tx.Exec(ctx, "SELECT "+fn+"($1)", subName); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func waitForSlotReleased(ctx context.Context, conn *pgxpool.Pool, slotName string) error {
	ctx, cancel := context.WithTimeout(ctx, slotReleaseTimeout)
	defer cancel()
	for {
		var active bool
		err := conn.QueryRow(ctx,
			"SELECT COALESCE((SELECT active FROM pg_replication_slots WHERE slot_name = $1), false)",
			slotName,
		).Scan(&active)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(slotReleasePollInterval):
		}
	}
}
