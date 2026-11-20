// internal/spock/version_checks.go
package spock

import (
	"context"
	"fmt"
	"strings"

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

// ValidateNativeSlotSync fails when a node with standbys relies on
// PostgreSQL's native slot sync to keep its Spock slots across a failover,
// but native slot sync can't work on it.
//
// A node relies on it when it runs Spock 6.0 or later on PostgreSQL 18 or
// later, where Spock no longer runs its own failover slot worker, or when
// sync_replication_slots is on (e.g. PostgreSQL 17 with
// pgEdge.nativeSlotSync: true), where Spock's worker yields to it. Native
// slot sync then needs:
//
//   - sync_replication_slots on;
//   - hot_standby_feedback on, which PostgreSQL requires on the standby;
//   - CNPG's replicationSlots.highAvailability enabled, for the standby's
//     physical slot;
//   - replicationSlots.highAvailability.synchronizeLogicalDecoding, so CNPG
//     puts dbname in the standby's primary_conninfo, which slot sync
//     requires, and manages synchronized_standby_slots, which keeps peers
//     from receiving changes the standby doesn't have yet.
//
// The chart sets all of them through pgEdge.nativeSlotSync; this catches the
// cases it can't, such as an image tag it doesn't recognise with
// nativeSlotSync: auto, nativeSlotSync: false, or settings overridden in a
// node's clusterSpec.
func ValidateNativeSlotSync(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	specs, err := cluster.HASpecs(ctx, cfg.Namespace, cfg.AppName)
	if err != nil {
		return fmt.Errorf("check CNPG cluster specs: %w", err)
	}
	for _, node := range cfg.Nodes {
		spec, ok := specs[fmt.Sprintf("%s-%s", cfg.AppName, node.Name)]
		if !ok || spec.Instances <= 1 {
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
		var syncSlots, hotStandbyFeedback string
		if err := conn.QueryRow(ctx, `
			SELECT COALESCE(current_setting('sync_replication_slots', true), 'off'),
			       current_setting('hot_standby_feedback')`,
		).Scan(&syncSlots, &hotStandbyFeedback); err != nil {
			return fmt.Errorf("check slot sync settings on %s: %w", node.Name, err)
		}
		if err := checkNativeSlotSync(node.Name, pgMajor, spockMajor, spec,
			syncSlots, hotStandbyFeedback); err != nil {
			return err
		}
	}
	return nil
}

// checkNativeSlotSync applies ValidateNativeSlotSync's rules to one node
// with more than one instance.
func checkNativeSlotSync(node string, pgMajor, spockMajor int, spec cluster.HASpec, syncSlots, hotStandbyFeedback string) error {
	required := needsNativeSlotSync(pgMajor, spockMajor)
	if !required && syncSlots != "on" {
		// Spock's own failover slot worker copies the slots.
		return nil
	}

	var problems []string
	if syncSlots != "on" {
		problems = append(problems, fmt.Sprintf("sync_replication_slots is %q", syncSlots))
	}
	if hotStandbyFeedback != "on" {
		problems = append(problems, fmt.Sprintf("hot_standby_feedback is %q", hotStandbyFeedback))
	}
	if !spec.HASlots {
		problems = append(problems, "CNPG's replicationSlots.highAvailability is disabled")
	}
	if !spec.SynchronizeLogicalDecoding {
		problems = append(problems, "replicationSlots.highAvailability.synchronizeLogicalDecoding is not enabled")
	}
	if len(problems) == 0 {
		return nil
	}

	why := "has sync_replication_slots on"
	if required {
		why = fmt.Sprintf("runs Spock %d on PostgreSQL %d", spockMajor, pgMajor)
	}
	return fmt.Errorf("node %s %s with %d instances, so its standbys only get copies of the "+
		"Spock slots through PostgreSQL's native slot sync, but %s. Without working slot "+
		"sync, a promoted standby would stop the other nodes replicating from it. Set "+
		"pgEdge.nativeSlotSync: true and remove any overrides of these settings, or set "+
		"replicationSlots.highAvailability.enabled and synchronizeLogicalDecoding, "+
		"sync_replication_slots: \"on\" and hot_standby_feedback: \"on\" in the node's "+
		"clusterSpec; see docs/usage/spock6.md",
		node, why, spec.Instances, strings.Join(problems, ", "))
}

// needsNativeSlotSync reports whether standbys can only get copies of the
// Spock slots through PostgreSQL's native slot sync.
func needsNativeSlotSync(pgMajor, spockMajor int) bool {
	return pgMajor >= 18 && spockMajor >= 6
}
