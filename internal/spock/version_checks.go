// internal/spock/version_checks.go
package spock

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/cluster"
	"github.com/pgEdge/pgedge-helm/internal/config"
)

// ValidateAddNodeVersions rejects adding a node (bootstrap.mode: spock) that
// runs a different Spock major version than any other node. Spock 5.0.x and
// 6.0 nodes can replicate to each other during a rolling upgrade, but adding
// a node across major versions isn't supported. It runs before anything is
// changed, and reads the version of each node's Spock binaries so it doesn't
// depend on the extension already existing.
func ValidateAddNodeVersions(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	adding := false
	for _, node := range cfg.Nodes {
		if node.Bootstrap.Mode == "spock" {
			adding = true
		}
	}
	if !adding {
		return nil
	}
	versions := make(map[string]int, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		major, err := availableSpockMajor(ctx, conns[node.Name])
		if err != nil {
			return fmt.Errorf("check spock version on %s: %w", node.Name, err)
		}
		versions[node.Name] = major
	}
	return checkAddNodeVersions(cfg.Nodes, versions)
}

func checkAddNodeVersions(nodes []config.Node, versions map[string]int) error {
	for _, node := range nodes {
		if node.Bootstrap.Mode != "spock" {
			continue
		}
		for _, peer := range nodes {
			if peer.Name == node.Name || versions[peer.Name] == versions[node.Name] {
				continue
			}
			return fmt.Errorf("node %s runs Spock %d but node %s runs Spock %d: adding a node "+
				"with a different Spock major version is not supported; use the same image "+
				"major versions on every node, or upgrade the existing nodes first",
				node.Name, versions[node.Name], peer.Name, versions[peer.Name])
		}
	}
	return nil
}

// ValidateNativeSlotSync fails when a node with standbys needs PostgreSQL's
// native slot sync to keep its Spock slots across a failover but doesn't
// have it. That is the case for Spock 6.0 or later on PostgreSQL 18 or
// later, where Spock no longer runs its own failover slot worker: without
// sync_replication_slots, a promoted standby has no Spock slots and the
// other nodes stop replicating from it. The chart turns it on through
// pgEdge.nativeSlotSync; this catches the cases it can't, such as an image
// tag it doesn't recognise with nativeSlotSync: auto, nativeSlotSync: false,
// or CNPG's HA replication slots turned off.
func ValidateNativeSlotSync(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	counts, err := cluster.InstanceCounts(ctx, cfg.Namespace, cfg.AppName)
	if err != nil {
		return fmt.Errorf("check instance counts: %w", err)
	}
	for _, node := range cfg.Nodes {
		instances := counts[fmt.Sprintf("%s-%s", cfg.AppName, node.Name)]
		if instances <= 1 {
			// A single instance, or a node not managed by this release.
			continue
		}
		conn := conns[node.Name]
		pgMajor, err := serverMajor(ctx, conn)
		if err != nil {
			return fmt.Errorf("check postgres version on %s: %w", node.Name, err)
		}
		spockMajor, err := availableSpockMajor(ctx, conn)
		if err != nil {
			return fmt.Errorf("check spock version on %s: %w", node.Name, err)
		}
		if !needsNativeSlotSync(pgMajor, spockMajor) {
			continue
		}
		var syncSlots string
		if err := conn.QueryRow(ctx,
			"SELECT COALESCE(current_setting('sync_replication_slots', true), 'off')",
		).Scan(&syncSlots); err != nil {
			return fmt.Errorf("check sync_replication_slots on %s: %w", node.Name, err)
		}
		if syncSlots != "on" {
			return fmt.Errorf("node %s runs Spock %d on PostgreSQL %d with %d instances, but "+
				"sync_replication_slots is %q, so its standbys get no copies of the Spock slots "+
				"and a promoted standby would stop the other nodes replicating from it. Set "+
				"pgEdge.nativeSlotSync: true (and keep CNPG's replicationSlots.highAvailability "+
				"enabled), or set replicationSlots.highAvailability.synchronizeLogicalDecoding "+
				"and sync_replication_slots in the node's clusterSpec; see docs/usage/spock6.md",
				node.Name, spockMajor, pgMajor, instances, syncSlots)
		}
	}
	return nil
}

// needsNativeSlotSync reports whether standbys can only get copies of the
// Spock slots through PostgreSQL's native slot sync.
func needsNativeSlotSync(pgMajor, spockMajor int) bool {
	return pgMajor >= 18 && spockMajor >= 6
}
