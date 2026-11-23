// internal/spock/standby_slots.go
package spock

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// standbySlotNamesQuery returns the physical slots named in
// spock.pg_standby_slot_names that don't exist on the node. It returns no
// rows when the setting is empty or unknown to the installed Spock.
const standbySlotNamesQuery = `
	SELECT name
	FROM unnest(string_to_array(
	       COALESCE(current_setting('spock.pg_standby_slot_names', true), ''), ',')) AS raw(name_raw),
	     LATERAL (SELECT btrim(name_raw) AS name) n
	WHERE n.name <> ''
	  AND NOT EXISTS (SELECT 1 FROM pg_replication_slots s WHERE s.slot_name = n.name)`

// CheckStandbySlotNames warns about physical slots named in
// spock.pg_standby_slot_names that don't exist on a node.
//
// From Spock 6.0 on PostgreSQL 18 the setting takes effect: with the default
// spock.standby_slots_min_confirmed = -1, every logical walsender waits for
// all named slots until wal_sender_timeout, so a name without a slot stalls
// replication from that node. The chart never sets it, but it can be set in
// the values or carried over from elsewhere. This only reports the problem,
// since the fix (creating the slot or clearing the setting) is the user's
// call. A failed check is logged and doesn't stop the job.
func CheckStandbySlotNames(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) {
	for _, node := range cfg.Nodes {
		missing, err := missingStandbySlots(ctx, conns[node.Name])
		if err != nil {
			slog.Warn("could not check spock.pg_standby_slot_names", "node", node.Name, "error", err)
			continue
		}
		if len(missing) > 0 {
			slog.Warn("spock.pg_standby_slot_names names physical slots that do not exist; "+
				"logical replication from this node can stall until they are created or the setting is cleared",
				"node", node.Name, "missing_slots", strings.Join(missing, ","))
		}
	}
}

func missingStandbySlots(ctx context.Context, conn *pgxpool.Pool) ([]string, error) {
	rows, err := conn.Query(ctx, standbySlotNamesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missing []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		missing = append(missing, name)
	}
	return missing, rows.Err()
}
