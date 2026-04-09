// internal/spock/desired.go
package spock

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// Resource type constants for Spock resources.
const (
	ResourceTypeUser            = "spock.user"
	ResourceTypeNode            = "spock.node"
	ResourceTypeReplicationSlot = "spock.replicationslot"
	ResourceTypeSubscription    = "spock.subscription"
)

// ComputeDesired builds the full resource graph from node config.
func ComputeDesired(cfg *config.Config, conns map[string]*pgxpool.Pool) map[resource.Identifier]resource.Resource {
	resources := make(map[resource.Identifier]resource.Resource)

	for _, node := range cfg.Nodes {
		u := NewPgEdgeUser(node, cfg.DBName, cfg.AdminUser, cfg.PgEdgeUser, conns[node.Name])
		resources[u.Identifier()] = u

		n := NewSpockNode(node, cfg.DBName, cfg.PgEdgeUser, conns[node.Name])
		resources[n.Identifier()] = n
	}

	// Subscriptions: every pair, bidirectional
	for _, src := range cfg.Nodes {
		for _, dst := range cfg.Nodes {
			if src.Name == dst.Name {
				continue
			}
			slot := NewReplicationSlot(src.Name, dst.Name, cfg.DBName, conns[src.Name])
			resources[slot.Identifier()] = slot

			sync := dst.Bootstrap.Mode == "spock" && dst.Bootstrap.SourceNode == src.Name
			s := NewSubscription(src, dst, cfg.DBName, cfg.AdminUser, cfg.PgEdgeUser, sync, conns[dst.Name])
			resources[s.Identifier()] = s
		}
	}

	return resources
}
