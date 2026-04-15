// internal/spock/desired.go
package spock

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// Resource type constants for Spock resources.
const (
	ResourceTypeUser                          = "spock.user"
	ResourceTypeNode                          = "spock.node"
	ResourceTypeReplicationSlot               = "spock.replicationslot"
	ResourceTypeSubscription                  = "spock.subscription"
	ResourceTypeReplicationSlotCreate         = "spock.replication_slot_create"
	ResourceTypeSyncEvent                     = "spock.sync_event"
	ResourceTypeWaitForSyncEvent              = "spock.wait_for_sync_event"
	ResourceTypeLagTrackerCommitTS            = "spock.lag_tracker_commit_ts"
	ResourceTypeReplicationSlotAdvanceFromCTS = "spock.replication_slot_advance_from_cts"
	ResourceTypeDisabledSubscription          = "spock.disabled_subscription"
)

// ComputeDesired builds the full resource graph from node config.
func ComputeDesired(cfg *config.Config, conns map[string]*pgxpool.Pool) map[resource.Identifier]resource.Resource {
	resources := make(map[resource.Identifier]resource.Resource)

	// Users + Nodes
	for _, node := range cfg.Nodes {
		u := NewPgEdgeUser(node, cfg.DBName, cfg.PgEdgeUser, conns[node.Name])
		resources[u.Identifier()] = u

		n := NewSpockNode(node, cfg.DBName, cfg.PgEdgeUser, conns[node.Name])
		resources[n.Identifier()] = n
	}

	// Identify new nodes needing populate
	newNodes := map[string]config.Node{}
	for _, node := range cfg.Nodes {
		if node.Bootstrap.Mode == "spock" && node.Bootstrap.SourceNode != "" {
			newNodes[node.Name] = node
		}
	}

	// Emit populate resources and collect per-node peer deps
	peerDeps := map[string][]resource.Identifier{}
	for _, newNode := range newNodes {
		deps := addPopulateResources(resources, cfg, newNode, conns)
		peerDeps[newNode.Name] = deps
	}

	// End-state subscriptions: every pair, bidirectional
	for _, src := range cfg.Nodes {
		for _, dst := range cfg.Nodes {
			if src.Name == dst.Name {
				continue
			}

			slot := NewReplicationSlot(src.Name, dst.Name, cfg.DBName, conns[src.Name])
			resources[slot.Identifier()] = slot

			// Source→new subscription: created with sync=true, extra deps on peer waits
			if newNode, isNewDst := newNodes[dst.Name]; isNewDst && src.Name == newNode.Bootstrap.SourceNode {
				s := NewSubscription(src, dst, cfg.DBName, cfg.PgEdgeUser, true, conns[dst.Name], peerDeps[dst.Name]...)
				resources[s.Identifier()] = s
				continue
			}

			// Compute extraDeps for other subscriptions involving new nodes
			var extraDeps []resource.Identifier

			if _, isNewDst := newNodes[dst.Name]; isNewDst {
				// Peer→new: wait for slot advance
				extraDeps = append(extraDeps, resource.Identifier{
					Type: ResourceTypeReplicationSlotAdvanceFromCTS,
					ID:   fmt.Sprintf("%s_%s", src.Name, dst.Name),
				})
			}

			if newNode, isNewSrc := newNodes[src.Name]; isNewSrc {
				// New→existing: wait for source sync completion
				extraDeps = append(extraDeps, resource.Identifier{
					Type: ResourceTypeWaitForSyncEvent,
					ID:   fmt.Sprintf("%s_%s", newNode.Bootstrap.SourceNode, src.Name),
				})
			}

			s := NewSubscription(src, dst, cfg.DBName, cfg.PgEdgeUser, false, conns[dst.Name], extraDeps...)
			resources[s.Identifier()] = s
		}
	}

	return resources
}

// addPopulateResources emits the populate resource chain for a new node.
// Returns the identifiers of peer WaitForSyncEvent resources (used to
// gate the source→new subscription).
func addPopulateResources(
	resources map[resource.Identifier]resource.Resource,
	cfg *config.Config,
	newNode config.Node,
	conns map[string]*pgxpool.Pool,
) []resource.Identifier {
	sourceNode := newNode.Bootstrap.SourceNode
	var peerWaitForSync []resource.Identifier

	for _, peer := range cfg.Nodes {
		if peer.Name == newNode.Name || peer.Name == sourceNode {
			continue
		}

		slotCreate := NewReplicationSlotCreate(peer.Name, newNode.Name, cfg.DBName, conns[peer.Name])
		resources[slotCreate.Identifier()] = slotCreate

		// Create disabled subscription from peer→new early in populate.
		// This registers the subscription in Spock metadata so the lag tracker
		// can populate. The end-state Subscription enables it after slot advance.
		disabledSub := NewDisabledSubscription(peer, newNode, cfg.DBName, cfg.PgEdgeUser, conns[newNode.Name])
		resources[disabledSub.Identifier()] = disabledSub

		peerSyncEvt := NewSyncEvent(peer.Name, sourceNode, conns[peer.Name],
			slotCreate.Identifier(),
		)
		resources[peerSyncEvt.Identifier()] = peerSyncEvt

		peerWaitEvt := NewWaitForSyncEvent(peer.Name, sourceNode, peerSyncEvt, conns[sourceNode])
		resources[peerWaitEvt.Identifier()] = peerWaitEvt
		peerWaitForSync = append(peerWaitForSync, peerWaitEvt.Identifier())

		lagTracker := NewLagTrackerCommitTimestamp(peer.Name, newNode.Name, conns[newNode.Name],
			resource.Identifier{Type: ResourceTypeWaitForSyncEvent, ID: fmt.Sprintf("%s_%s", sourceNode, newNode.Name)},
		)
		resources[lagTracker.Identifier()] = lagTracker

		slotAdvance := NewReplicationSlotAdvanceFromCTS(peer.Name, newNode.Name, cfg.DBName, lagTracker, conns[peer.Name])
		resources[slotAdvance.Identifier()] = slotAdvance
	}

	// Source sync event + wait
	srcSyncEvt := NewSyncEvent(sourceNode, newNode.Name, conns[sourceNode])
	resources[srcSyncEvt.Identifier()] = srcSyncEvt

	srcWaitEvt := NewWaitForSyncEvent(sourceNode, newNode.Name, srcSyncEvt, conns[newNode.Name])
	resources[srcWaitEvt.Identifier()] = srcWaitEvt

	return peerWaitForSync
}
