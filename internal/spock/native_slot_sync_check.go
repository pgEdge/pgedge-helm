// internal/spock/native_slot_sync_check.go
package spock

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/cluster"
	"github.com/pgEdge/pgedge-helm/internal/config"
)

// ValidateNativeSlotSync fails loudly if a node needs Postgres's native
// replication-slot sync (Spock 6+ on PostgreSQL 17+, see
// needsNativeFailoverSlots) and actually has standbys, but
// sync_replication_slots isn't on — instead of leaving a promoted standby
// silently missing its Spock slot. See pgEdge.nativeSlotSync in values.yaml.
func ValidateNativeSlotSync(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	counts, err := cluster.InstanceCounts(ctx, cfg.Namespace, cfg.AppName)
	if err != nil {
		return fmt.Errorf("check instance counts: %w", err)
	}

	for _, node := range cfg.Nodes {
		conn := conns[node.Name]
		needsIt, err := needsNativeFailoverSlots(ctx, conn)
		if err != nil {
			return fmt.Errorf("check native failover slot requirement on %s: %w", node.Name, err)
		}
		if !needsIt {
			continue
		}
		instances := counts[fmt.Sprintf("%s-%s", cfg.AppName, node.Name)]
		if instances <= 1 {
			continue
		}

		var syncReplicationSlots string
		if err := conn.QueryRow(ctx, "SHOW sync_replication_slots").Scan(&syncReplicationSlots); err != nil {
			return fmt.Errorf("check sync_replication_slots on %s: %w", node.Name, err)
		}
		if syncReplicationSlots != "on" {
			return fmt.Errorf(
				"node %s runs Spock 6+ on PostgreSQL 17+ with %d instances, but sync_replication_slots is %q: "+
					"a promoted standby would have no Spock replication slot. Set pgEdge.nativeSlotSync: true "+
					"(or clusterSpec.postgresql.parameters.sync_replication_slots: \"on\" directly) if auto-detection "+
					"didn't recognize this node's image tag",
				node.Name, instances, syncReplicationSlots,
			)
		}
	}
	return nil
}
