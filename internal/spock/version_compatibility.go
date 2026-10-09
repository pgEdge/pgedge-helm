// internal/spock/version_compatibility.go
package spock

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// ValidateVersionCompatibility rejects an add-node Spock major-version mismatch, which the current preview image can't safely bridge.
// Runs before any reset step, checking the version available to install rather than one requiring the extension to already exist.
func ValidateVersionCompatibility(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	versions := make(map[string]uint64, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		major, err := availableSpockMajorVersion(ctx, conns[node.Name])
		if err != nil {
			return fmt.Errorf("check spock version on %s: %w", node.Name, err)
		}
		versions[node.Name] = major
	}
	return validateVersionCompatibility(cfg.Nodes, versions)
}

// validateVersionCompatibility is the pure comparison logic behind
// ValidateVersionCompatibility, separated out for testing without a
// database connection.
func validateVersionCompatibility(nodes []config.Node, versions map[string]uint64) error {
	for _, node := range nodes {
		if node.Bootstrap.Mode != "spock" {
			continue
		}
		newMajor := versions[node.Name]
		for _, peer := range nodes {
			if peer.Name == node.Name {
				continue
			}
			if peerMajor := versions[peer.Name]; newMajor != peerMajor {
				return fmt.Errorf(
					"node %s (Spock %d) cannot join peer %s (Spock %d): "+
						"add-node across different Spock major versions is not currently supported",
					node.Name, newMajor, peer.Name, peerMajor,
				)
			}
		}
	}
	return nil
}
