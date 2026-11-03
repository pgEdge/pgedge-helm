//go:build unit

package unit

import "testing"

func TestNativeSlotSyncAutoDetectsFromImageTag(t *testing.T) {
	objects := renderTemplate(t, "native-slot-sync-auto-values.yaml")
	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	if got := getNestedString(n1, "spec", "postgresql", "parameters", "sync_replication_slots"); got != "on" {
		t.Errorf("sync_replication_slots: got %q, want \"on\"", got)
	}
	if got := getNestedBool(n1, "spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding"); !got {
		t.Error("replicationSlots.highAvailability.synchronizeLogicalDecoding: got false, want true")
	}
}

func TestNativeSlotSyncDefaultSpock5ImageNotEnabled(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	if got := getNestedString(n1, "spec", "postgresql", "parameters", "sync_replication_slots"); got != "" {
		t.Errorf("sync_replication_slots: got %q, want unset (chart default image is Spock 5)", got)
	}
	if got := getNestedBool(n1, "spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding"); got {
		t.Error("replicationSlots.highAvailability.synchronizeLogicalDecoding: should not be set for the default Spock 5 image")
	}
}

func TestNativeSlotSyncExplicitFalseOverridesAutoDetection(t *testing.T) {
	objects := renderTemplate(t, "native-slot-sync-explicit-false-values.yaml")
	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	if got := getNestedString(n1, "spec", "postgresql", "parameters", "sync_replication_slots"); got != "" {
		t.Errorf("sync_replication_slots: got %q, want unset — nativeSlotSync: false must suppress auto-detection even for a Spock 6 image tag", got)
	}
}

func TestNativeSlotSyncExplicitTrueForcesUnparseableTag(t *testing.T) {
	objects := renderTemplate(t, "native-slot-sync-explicit-true-values.yaml")
	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	if got := getNestedString(n1, "spec", "postgresql", "parameters", "sync_replication_slots"); got != "on" {
		t.Errorf("sync_replication_slots: got %q, want \"on\" — nativeSlotSync: true must force-enable even for a digest-pinned image tag auto-detection can't parse", got)
	}
}
