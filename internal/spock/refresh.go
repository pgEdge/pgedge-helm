// internal/spock/refresh.go
package spock

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// RefreshActual refreshes all resources in the desired set to populate their Status,
// discovers orphan nodes, subscriptions, and replication slots from PostgreSQL catalogs,
// then cross-references subscriptions with their provider-side slots.
// Returns the combined "actual" map of everything that exists.
func RefreshActual(
	ctx context.Context,
	cfg *config.Config,
	conns map[string]*pgxpool.Pool,
	names SlotNames,
	desired map[resource.Identifier]resource.Resource,
) (map[resource.Identifier]resource.Resource, error) {
	actual := make(map[resource.Identifier]resource.Resource)

	for id, r := range desired {
		if err := r.Refresh(ctx); err != nil {
			return nil, fmt.Errorf("refresh %s/%s: %w", id.Type, id.ID, err)
		}
		if r.Status().Exists {
			actual[id] = r
		}
	}

	discoverOrphans(ctx, cfg, conns, names, actual)
	checkSlotHealth(actual)

	return actual, nil
}

// checkSlotHealth marks subscriptions as NeedsRecreate when their provider-side
// replication slot is missing. This detects broken replication after PG upgrades
// or database restores where slots are lost but subscriptions remain.
func checkSlotHealth(actual map[resource.Identifier]resource.Resource) {
	for id, r := range actual {
		if id.Type != ResourceTypeSubscription {
			continue
		}
		sub, ok := r.(*Subscription)
		if !ok {
			continue
		}
		slotID := resource.Identifier{Type: ResourceTypeReplicationSlot, ID: sub.replicationSlotName()}
		if _, slotExists := actual[slotID]; !slotExists {
			sub.status = resource.Status{
				Exists:        true,
				NeedsRecreate: true,
				Reason:        "provider-side replication slot missing",
			}
			slog.Warn("subscription missing provider slot",
				"sub", sub.subName(), "slot", sub.replicationSlotName())
		}
	}
}

// discoverOrphans queries each surviving node for Spock nodes, subscriptions,
// and replication slots not in the config, adding them to the actual map.
func discoverOrphans(
	ctx context.Context,
	cfg *config.Config,
	conns map[string]*pgxpool.Pool,
	names SlotNames,
	actual map[resource.Identifier]resource.Resource,
) {
	configNames := make([]string, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		configNames[i] = n.Name
	}

	// Expected slot names for orphan slot detection.
	expectedSlots := make(map[string]bool, len(names))
	for _, name := range names {
		expectedSlots[name] = true
	}

	for _, node := range cfg.Nodes {
		conn := conns[node.Name]

		discoverOrphanNodes(ctx, cfg, conn, node, configNames, actual)
		discoverOrphanSlots(ctx, cfg, conn, node, expectedSlots, actual)
	}
}

// discoverOrphanNodes finds Spock nodes not in the config on a surviving connection.
func discoverOrphanNodes(
	ctx context.Context,
	cfg *config.Config,
	conn *pgxpool.Pool,
	survivor config.Node,
	configNames []string,
	actual map[resource.Identifier]resource.Resource,
) {
	rows, err := conn.Query(ctx,
		"SELECT node_name FROM spock.node WHERE node_name != ALL($1::text[])", configNames)
	if err != nil {
		slog.Warn("query orphan nodes", "survivor", survivor.Name, "error", err)
		return
	}
	var orphanNames []string
	for rows.Next() {
		var orphanName string
		if err := rows.Scan(&orphanName); err != nil {
			continue
		}
		orphanNames = append(orphanNames, orphanName)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Warn("incomplete orphan node scan", "survivor", survivor.Name, "error", err)
	}

	for _, orphanName := range orphanNames {
		orphanCfg := config.Node{Name: orphanName}

		// Create one SpockNode per (orphan, survivor) pair so node_drop
		// runs on every survivor's connection. Use a survivor-scoped ID
		// to avoid collisions in the actual map.
		nodeID := resource.Identifier{
			Type: ResourceTypeNode,
			ID:   fmt.Sprintf("%s@%s", orphanName, survivor.Name),
		}
		n := NewSpockNode(orphanCfg, cfg.DBName, cfg.PgEdgeUser, conn)
		n.status = resource.Status{Exists: true}
		actual[nodeID] = n
		slog.Info("discovered orphan node", "orphan", orphanName, "survivor", survivor.Name)

		// Infer one orphan subscription per surviving connection.
		// The topology is fully meshed, so each surviving node had a subscription
		// from the orphan. sub_drop($1, true) is safe if it no longer exists.
		slotName, err := genSlotName(ctx, conn, cfg.DBName, orphanName, survivor.Name)
		if err != nil {
			slog.Warn("skip orphan subscription", "orphan", orphanName, "survivor", survivor.Name, "error", err)
			continue
		}
		sub := NewSubscription(orphanCfg, survivor, cfg.DBName, cfg.PgEdgeUser, slotName, false, conn)
		sub.status = resource.Status{Exists: true}
		actual[sub.Identifier()] = sub
		slog.Info("discovered orphan subscription", "sub", sub.subName(), "survivor", survivor.Name)
	}
}

// discoverOrphanSlots finds logical replication slots not matching any configured subscription.
// Only slots in the connected database are considered: Spock may also run in
// other databases on the same server, and their slots are not ours to drop.
func discoverOrphanSlots(
	ctx context.Context,
	cfg *config.Config,
	conn *pgxpool.Pool,
	survivor config.Node,
	expectedSlots map[string]bool,
	actual map[resource.Identifier]resource.Resource,
) {
	rows, err := conn.Query(ctx, `
		SELECT slot_name FROM pg_replication_slots
		 WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%'
		   AND database = current_database()`)
	if err != nil {
		slog.Warn("query replication slots", "survivor", survivor.Name, "error", err)
		return
	}

	for rows.Next() {
		var slotName string
		if err := rows.Scan(&slotName); err != nil {
			continue
		}
		if expectedSlots[slotName] {
			continue
		}

		slotID := resource.Identifier{Type: ResourceTypeReplicationSlot, ID: slotName}
		if _, exists := actual[slotID]; !exists {
			orphan := &ReplicationSlot{
				providerName: survivor.Name,
				name:         slotName,
				conn:         conn,
			}
			orphan.status = resource.Status{Exists: true}
			actual[slotID] = orphan
			slog.Info("discovered orphan replication slot", "slot", slotName, "survivor", survivor.Name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Warn("incomplete slot scan", "survivor", survivor.Name, "error", err)
	}
}
