// internal/spock/names_test.go
package spock

import (
	"strings"
	"testing"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// testSlotNames stands in for LoadSlotNames in tests. It builds the names
// Spock gives short node and database names ("spk_<db>_<provider>_<sub>").
// Spock shortens names longer than 16 bytes, which this does not do, so tests
// must use short names.
func testSlotNames(cfg *config.Config) SlotNames {
	names := SlotNames{}
	for _, src := range cfg.Nodes {
		for _, dst := range cfg.Nodes {
			if src.Name == dst.Name {
				continue
			}
			names[slotPair{src.Name, dst.Name}] = strings.ReplaceAll(
				"spk_"+cfg.DBName+"_"+src.Name+"_"+spockSubName(src.Name, dst.Name), "-", "_")
		}
	}
	return names
}

func TestSlotNamesLookup(t *testing.T) {
	names := SlotNames{
		{"node-alpha", "node-beta"}: "spk_app_node_alpha_sub_node57e0ceb",
	}
	if got, want := names.slot("node-alpha", "node-beta"), "spk_app_node_alpha_sub_node57e0ceb"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSlotNamesMissingPairPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for a pair with no loaded slot name")
		}
	}()
	SlotNames{}.slot("n1", "n2")
}

func TestComputeDesiredUsesLoadedSlotNames(t *testing.T) {
	cfg := &config.Config{
		DBName: "app",
		Nodes:  []config.Node{{Name: "node-alpha"}, {Name: "node-beta"}},
	}
	names := SlotNames{
		{"node-alpha", "node-beta"}: "spk_app_node_alpha_sub_node57e0ceb",
		{"node-beta", "node-alpha"}: "spk_app_node_beta_sub_node1234567",
	}
	resources := ComputeDesired(cfg, nil, names)
	for _, slot := range names {
		assertResource(t, resources, ResourceTypeReplicationSlot, slot)
	}
}
