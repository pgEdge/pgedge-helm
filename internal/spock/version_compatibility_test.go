// internal/spock/version_compatibility_test.go
package spock

import (
	"testing"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

func TestValidateVersionCompatibilityAllowsEqualVersionJoin(t *testing.T) {
	nodes := []config.Node{
		{Name: "n1"},
		{Name: "n2"},
		{Name: "n3", Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"}},
	}
	versions := map[string]uint64{"n1": 5, "n2": 5, "n3": 5}
	if err := validateVersionCompatibility(nodes, versions); err != nil {
		t.Errorf("expected equal-version join to succeed, got error: %v", err)
	}
}

func TestValidateVersionCompatibilityRejectsAnyMismatch(t *testing.T) {
	nodes := []config.Node{
		{Name: "n1"},
		{Name: "n2"},
		{Name: "n3", Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"}},
	}

	// Older node joining a newer cluster.
	versions := map[string]uint64{"n1": 6, "n2": 6, "n3": 5}
	if err := validateVersionCompatibility(nodes, versions); err == nil {
		t.Error("expected error for older node joining newer cluster, got nil")
	}

	// Newer node joining an older cluster — also rejected today; see the
	// doc comment on validateVersionCompatibility for why.
	versions = map[string]uint64{"n1": 5, "n2": 5, "n3": 6}
	if err := validateVersionCompatibility(nodes, versions); err == nil {
		t.Error("expected error for newer node joining older cluster, got nil")
	}
}

func TestValidateVersionCompatibilityIgnoresNonSpockBootstrap(t *testing.T) {
	nodes := []config.Node{
		{Name: "n1"},
		{Name: "n2", Bootstrap: config.NodeBootstrap{Mode: "cnpg", SourceNode: "n1"}},
	}
	versions := map[string]uint64{"n1": 6, "n2": 5}
	if err := validateVersionCompatibility(nodes, versions); err != nil {
		t.Errorf("cnpg-bootstrapped nodes should not be checked, got error: %v", err)
	}
}
