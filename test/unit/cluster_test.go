//go:build unit

package unit

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDistributedGeneratesClusterPerNode(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 3 {
		t.Fatalf("expected 3 Cluster resources, got %d", len(clusters))
	}

	expectedNames := []string{"pgedge-n1", "pgedge-n2", "pgedge-n3"}
	for i, name := range expectedNames {
		if clusters[i].GetName() != name {
			t.Errorf("cluster %d: expected name %q, got %q", i, name, clusters[i].GetName())
		}
	}
}

func TestDistributedNodeOrdinals(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	expected := map[string]string{
		"pgedge-n1": "1",
		"pgedge-n2": "2",
		"pgedge-n3": "3",
	}

	for _, cluster := range clusters {
		name := cluster.GetName()
		wantOrd := expected[name]

		snowflake := getNestedString(&cluster, "spec", "postgresql", "parameters", "snowflake.node")
		if snowflake != wantOrd {
			t.Errorf("%s: expected snowflake.node=%q, got %q", name, wantOrd, snowflake)
		}

		lolor := getNestedString(&cluster, "spec", "postgresql", "parameters", "lolor.node")
		if lolor != wantOrd {
			t.Errorf("%s: expected lolor.node=%q, got %q", name, wantOrd, lolor)
		}
	}
}

func TestDistributedMergesNodeClusterSpec(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")

	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	// n1 should have instances: 3 (node override)
	instances, found, _ := unstructured.NestedInt64(n1.Object, "spec", "instances")
	if !found || instances != 3 {
		t.Errorf("pgedge-n1: expected instances=3, got %d (found=%v)", instances, found)
	}

	// n2 should inherit default instances: 1
	n2 := findByKindAndName(objects, "Cluster", "pgedge-n2")
	if n2 == nil {
		t.Fatal("pgedge-n2 Cluster not found")
	}
	instances2, found2, _ := unstructured.NestedInt64(n2.Object, "spec", "instances")
	if !found2 || instances2 != 1 {
		t.Errorf("pgedge-n2: expected instances=1, got %d (found=%v)", instances2, found2)
	}
}

func TestSingleNodeSingleCluster(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster resource, got %d", len(clusters))
	}
	if clusters[0].GetName() != "pgedge-n1" {
		t.Errorf("expected name pgedge-n1, got %q", clusters[0].GetName())
	}
}

func TestCustomOrdinal(t *testing.T) {
	objects := renderTemplate(t, "custom-ordinal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	expected := map[string]string{
		"pgedge-n1": "10",
		"pgedge-n2": "20",
	}

	for _, cluster := range clusters {
		name := cluster.GetName()
		wantOrd := expected[name]

		snowflake := getNestedString(&cluster, "spec", "postgresql", "parameters", "snowflake.node")
		if snowflake != wantOrd {
			t.Errorf("%s: expected snowflake.node=%q, got %q", name, wantOrd, snowflake)
		}
	}
}

func TestClusterDefaultPostgreSQLParameters(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]

	params := map[string]string{
		"wal_level":                      "logical",
		"track_commit_timestamp":         "on",
		"spock.conflict_resolution":      "last_update_wins",
		"spock.enable_ddl_replication":   "on",
		"spock.include_ddl_repset":       "on",
		"spock.allow_ddl_from_functions": "on",
		"output_plugin_libraries":        "pgoutput, test_decoding, spock_output",
	}
	for key, want := range params {
		got := getNestedString(cluster, "spec", "postgresql", "parameters", key)
		if got != want {
			t.Errorf("expected %s=%q, got %q", key, want, got)
		}
	}
}

func TestOutputPluginLibrariesGlobalNullOverride(t *testing.T) {
	objects := renderTemplate(t, "output-plugin-libraries-global-null-values.yaml")
	cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if cluster == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	params, found, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
	if !found {
		t.Fatal("expected postgresql.parameters to be set")
	}
	if _, present := params["output_plugin_libraries"]; present {
		t.Errorf("expected output_plugin_libraries to be absent, got %v", params["output_plugin_libraries"])
	}
}

// Unlike the global override, a per-node clusterSpec merges onto the
// default via Sprig's mergeOverwrite, which copies a nil value into the
// map instead of deleting the key — so the key is present but nil here,
// not absent as in TestOutputPluginLibrariesGlobalNullOverride. This is
// NOT a safe substitute for the global override: confirmed live, the
// Kubernetes API stores that nil as an empty string rather than dropping
// the key, and a pre-patch PostgreSQL still rejects the parameter with an
// empty value exactly as it would any other value. A template-only test
// can't see that far, so this only pins the divergence at this layer.
func TestOutputPluginLibrariesPerNodeNullOverride(t *testing.T) {
	objects := renderTemplate(t, "output-plugin-libraries-null-values.yaml")
	cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if cluster == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	params, found, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
	if !found {
		t.Fatal("expected postgresql.parameters to be set")
	}
	value, present := params["output_plugin_libraries"]
	if !present {
		t.Fatal("expected output_plugin_libraries key to be present (as nil) in the rendered manifest, but it was absent")
	}
	if value != nil {
		t.Errorf("expected output_plugin_libraries to be nil, got %v", value)
	}
}

// spock.failover_slots_database (Spock 5.0.14+) is derived from the
// chart-wide initdb database, the database init-spock configures Spock in,
// so the failover slot worker decodes slot copies in the Spock database.
func TestFailoverSlotsDatabaseDefaultsToInitdbDatabase(t *testing.T) {
	for _, tc := range []struct {
		values string
		want   string
	}{
		{"single-node-minimal-values.yaml", "app"},
		{"custom-database-values.yaml", "mydb"},
	} {
		t.Run(tc.values, func(t *testing.T) {
			objects := renderTemplate(t, tc.values)
			cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
			if cluster == nil {
				t.Fatal("pgedge-n1 Cluster not found")
			}
			got := getNestedString(cluster, "spec", "postgresql", "parameters", "spock.failover_slots_database")
			if got != tc.want {
				t.Errorf("expected spock.failover_slots_database=%q, got %q", tc.want, got)
			}
		})
	}
}

// A node bootstrapped from a backup sets bootstrap.initdb to null. It must
// still render, and still get the chart-wide Spock database that init-spock
// configures on every node.
func TestFailoverSlotsDatabaseRecoveryNode(t *testing.T) {
	objects := renderTemplate(t, "recovery-node-values.yaml")
	for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
		cluster := findByKindAndName(objects, "Cluster", name)
		if cluster == nil {
			t.Fatalf("%s Cluster not found", name)
		}
		got := getNestedString(cluster, "spec", "postgresql", "parameters", "spock.failover_slots_database")
		if got != "app" {
			t.Errorf("%s: expected spock.failover_slots_database=%q, got %q", name, "app", got)
		}
	}
}

func TestFailoverSlotsDatabaseExplicitOverride(t *testing.T) {
	objects := renderTemplate(t, "failover-slots-database-override-values.yaml")
	cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if cluster == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}

	params, found, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
	if !found {
		t.Fatal("expected postgresql.parameters to be set")
	}
	value, present := params["spock.failover_slots_database"]
	if !present {
		t.Fatal("expected spock.failover_slots_database to be present")
	}
	if value != "" {
		t.Errorf("expected explicit spock.failover_slots_database=\"\" to be kept, got %v", value)
	}
}

// An override on a single node's clusterSpec applies to that node only; the
// chart's default still applies to the other nodes.
func TestFailoverSlotsDatabasePerNodeOverride(t *testing.T) {
	objects := renderTemplate(t, "failover-slots-database-per-node-override-values.yaml")

	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if n1 == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}
	params, found, _ := unstructured.NestedMap(n1.Object, "spec", "postgresql", "parameters")
	if !found {
		t.Fatal("expected postgresql.parameters to be set on pgedge-n1")
	}
	value, present := params["spock.failover_slots_database"]
	if !present {
		t.Fatal("expected spock.failover_slots_database to be present on pgedge-n1")
	}
	if value != "" {
		t.Errorf("pgedge-n1: expected per-node override \"\" to be kept, got %v", value)
	}

	n2 := findByKindAndName(objects, "Cluster", "pgedge-n2")
	if n2 == nil {
		t.Fatal("pgedge-n2 Cluster not found")
	}
	if got := getNestedString(n2, "spec", "postgresql", "parameters", "spock.failover_slots_database"); got != "app" {
		t.Errorf("pgedge-n2: expected spock.failover_slots_database=%q, got %q", "app", got)
	}
}

func TestClusterDefaultBootstrapConfig(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]

	// initdb.database = app
	db := getNestedString(cluster, "spec", "bootstrap", "initdb", "database")
	if db != "app" {
		t.Errorf("expected bootstrap.initdb.database=app, got %q", db)
	}

	// initdb.owner = app
	owner := getNestedString(cluster, "spec", "bootstrap", "initdb", "owner")
	if owner != "app" {
		t.Errorf("expected bootstrap.initdb.owner=app, got %q", owner)
	}

	// postInitApplicationSQL contains CREATE EXTENSION spock
	sqlSlice, found, _ := unstructured.NestedStringSlice(cluster.Object,
		"spec", "bootstrap", "initdb", "postInitApplicationSQL")
	if !found || len(sqlSlice) == 0 {
		t.Fatal("expected postInitApplicationSQL to be set")
	}
	foundSpock := false
	for _, s := range sqlSlice {
		if strings.Contains(s, "CREATE EXTENSION spock") {
			foundSpock = true
			break
		}
	}
	if !foundSpock {
		t.Errorf("expected postInitApplicationSQL to contain 'CREATE EXTENSION spock', got %v", sqlSlice)
	}
}

func TestClusterDefaultManagedRoles(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]
	roles, found, _ := unstructured.NestedSlice(cluster.Object, "spec", "managed", "roles")
	if !found || len(roles) == 0 {
		t.Fatal("expected managed.roles to be set")
	}

	adminRole := roles[0].(map[string]interface{})
	if adminRole["name"] != "admin" {
		t.Errorf("expected first role name=admin, got %v", adminRole["name"])
	}
	if adminRole["superuser"] != true {
		t.Errorf("expected admin role superuser=true, got %v", adminRole["superuser"])
	}
	if adminRole["login"] != true {
		t.Errorf("expected admin role login=true, got %v", adminRole["login"])
	}
}

func TestClusterDefaultPgHBA(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]
	hba, found, _ := unstructured.NestedStringSlice(cluster.Object, "spec", "postgresql", "pg_hba")
	if !found || len(hba) == 0 {
		t.Fatal("expected pg_hba to be set")
	}

	expected := []string{
		"hostssl app pgedge 0.0.0.0/0 cert",
		"hostssl app admin 0.0.0.0/0 cert",
		"hostssl app app 0.0.0.0/0 cert",
		"hostssl all streaming_replica all cert map=cnpg_streaming_replica",
	}
	for _, want := range expected {
		found := false
		for _, got := range hba {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("pg_hba missing rule: %s", want)
		}
	}
}

func TestClusterDefaultSharedPreloadLibraries(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]
	libs, found, _ := unstructured.NestedStringSlice(cluster.Object, "spec", "postgresql", "shared_preload_libraries")
	if !found || len(libs) == 0 {
		t.Fatal("expected shared_preload_libraries to be set")
	}

	for _, want := range []string{"pg_stat_statements", "snowflake", "spock"} {
		found := false
		for _, got := range libs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("shared_preload_libraries missing %s, got %v", want, libs)
		}
	}
}

func TestClusterDefaultProjectedVolumeTemplate(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]
	sources, found, _ := unstructured.NestedSlice(cluster.Object,
		"spec", "projectedVolumeTemplate", "sources")
	if !found || len(sources) == 0 {
		t.Fatal("expected projectedVolumeTemplate.sources to be set")
	}

	source := sources[0].(map[string]interface{})
	secret, ok := source["secret"].(map[string]interface{})
	if !ok {
		t.Fatal("expected first source to be a secret")
	}
	if secret["name"] != "pgedge-client-cert" {
		t.Errorf("expected secret name=pgedge-client-cert, got %v", secret["name"])
	}
}

func TestClusterDefaultImagePullPolicy(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	policy := getNestedString(&clusters[0], "spec", "imagePullPolicy")
	if policy != "Always" {
		t.Errorf("expected imagePullPolicy=Always, got %q", policy)
	}
}

func TestClusterDefaultImageName(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	imageName := getNestedString(&clusters[0], "spec", "imageName")
	expected := "ghcr.io/pgedge/pgedge-postgres:18-spock5-standard"
	if imageName != expected {
		t.Errorf("expected imageName=%q, got %q", expected, imageName)
	}
}

// Spock 6.0 on PostgreSQL 18 has no failover slot worker of its own, so the
// chart turns on CNPG's logical decoding slot sync for those images only.
func TestNativeSlotSyncForSpock6OnPG18(t *testing.T) {
	objects := renderTemplate(t, "spock6-native-slot-sync-values.yaml")
	for _, tc := range []struct {
		cluster string
		want    bool
	}{
		{"pgedge-n1", true},
		{"pgedge-n2", true},
		{"pgedge-n3", false},
		{"pgedge-n4", false},
		{"pgedge-n5", false},
		{"pgedge-n6", false},
		{"pgedge-n7", false},
	} {
		t.Run(tc.cluster, func(t *testing.T) {
			cluster := findByKindAndName(objects, "Cluster", tc.cluster)
			if cluster == nil {
				t.Fatalf("%s Cluster not found", tc.cluster)
			}
			sync, _, _ := unstructured.NestedBool(cluster.Object,
				"spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding")
			if sync != tc.want {
				t.Errorf("synchronizeLogicalDecoding = %v, want %v", sync, tc.want)
			}
			params, _, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
			value, present := params["sync_replication_slots"]
			if tc.want {
				if value != "on" {
					t.Errorf("sync_replication_slots = %v, want \"on\"", value)
				}
				if params["hot_standby_feedback"] != "on" {
					t.Errorf("hot_standby_feedback = %v, want \"on\"", params["hot_standby_feedback"])
				}
			} else if present {
				t.Errorf("sync_replication_slots should not be set, got %v", value)
			}
		})
	}

	// Turning it on for one node must not leak into the others.
	n5 := findByKindAndName(objects, "Cluster", "pgedge-n5")
	if enabled, found, _ := unstructured.NestedBool(n5.Object,
		"spec", "replicationSlots", "highAvailability", "enabled"); !found || enabled {
		t.Errorf("pgedge-n5: expected highAvailability.enabled=false to be kept")
	}
	n3 := findByKindAndName(objects, "Cluster", "pgedge-n3")
	if _, found, _ := unstructured.NestedMap(n3.Object, "spec", "replicationSlots"); found {
		t.Errorf("pgedge-n3: expected no replicationSlots section")
	}
}

func TestNativeSlotSyncRespectsUserSettings(t *testing.T) {
	objects := renderTemplate(t, "spock6-native-slot-sync-override-values.yaml")

	n1 := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if sync, found, _ := unstructured.NestedBool(n1.Object,
		"spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding"); !found || sync {
		t.Errorf("pgedge-n1: expected explicit synchronizeLogicalDecoding=false to be kept")
	}
	if params, _, _ := unstructured.NestedMap(n1.Object, "spec", "postgresql", "parameters"); params["sync_replication_slots"] != nil {
		t.Errorf("pgedge-n1: sync_replication_slots should not be set, got %v", params["sync_replication_slots"])
	}

	n2 := findByKindAndName(objects, "Cluster", "pgedge-n2")
	if got := getNestedString(n2, "spec", "postgresql", "parameters", "sync_replication_slots"); got != "off" {
		t.Errorf("pgedge-n2: expected explicit sync_replication_slots=off to be kept, got %q", got)
	}
	if _, found, _ := unstructured.NestedBool(n2.Object,
		"spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding"); found {
		t.Errorf("pgedge-n2: synchronizeLogicalDecoding should not be set")
	}
}

// The default image stays on Spock 5 until Spock 6.0 is GA.
func TestDefaultImageHasNoNativeSlotSync(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if cluster == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}
	if _, found, _ := unstructured.NestedMap(cluster.Object, "spec", "replicationSlots"); found {
		t.Error("expected no replicationSlots section for the default Spock 5 image")
	}
	params, _, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
	if _, present := params["sync_replication_slots"]; present {
		t.Error("expected sync_replication_slots to be absent for the default Spock 5 image")
	}
}

// Spock 6.0 settings, including the ones that now take units, pass schema
// validation and render as given. The chart never sets
// spock.pg_standby_slot_names, which stalls replication on PostgreSQL 18 with
// Spock 6.0 when it names a missing slot.
func TestSpock6SettingsRender(t *testing.T) {
	objects := renderTemplate(t, "spock6-guc-units-values.yaml")
	cluster := findByKindAndName(objects, "Cluster", "pgedge-n1")
	if cluster == nil {
		t.Fatal("pgedge-n1 Cluster not found")
	}
	for key, want := range map[string]string{
		"spock.restart_delay_default":      "5s",
		"spock.restart_delay_on_exception": "1min",
		"spock.output_delay":               "0",
		"spock.sync_timeout":               "2h",
		"spock.event_history_size":         "4096",
		"spock.non_conflicting_inserts":    "off",
		"output_plugin_libraries":          "pgoutput, test_decoding, spock_output",
	} {
		if got := getNestedString(cluster, "spec", "postgresql", "parameters", key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	params, _, _ := unstructured.NestedMap(cluster.Object, "spec", "postgresql", "parameters")
	if _, present := params["spock.pg_standby_slot_names"]; present {
		t.Error("the chart must not set spock.pg_standby_slot_names")
	}
}

// nativeSlotSyncOn reports whether a rendered Cluster has native slot sync on.
func nativeSlotSyncOn(t *testing.T, objects []unstructured.Unstructured, name string) bool {
	t.Helper()
	cluster := findByKindAndName(objects, "Cluster", name)
	if cluster == nil {
		t.Fatalf("%s Cluster not found", name)
	}
	sync, _, _ := unstructured.NestedBool(cluster.Object,
		"spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding")
	param := getNestedString(cluster, "spec", "postgresql", "parameters", "sync_replication_slots")
	if sync != (param == "on") {
		t.Errorf("%s: synchronizeLogicalDecoding=%v but sync_replication_slots=%q", name, sync, param)
	}
	return sync
}

func TestNativeSlotSyncTrueForcesOn(t *testing.T) {
	objects := renderTemplate(t, "native-slot-sync-true-values.yaml")
	for name, want := range map[string]bool{
		"pgedge-n1": true,  // digest image
		"pgedge-n2": true,  // PostgreSQL 17
		"pgedge-n3": false, // HA replication slots disabled
		"pgedge-n4": false, // single instance
	} {
		if got := nativeSlotSyncOn(t, objects, name); got != want {
			t.Errorf("%s: native slot sync = %v, want %v", name, got, want)
		}
	}
}

func TestNativeSlotSyncFalseKeepsOff(t *testing.T) {
	objects := renderTemplate(t, "native-slot-sync-false-values.yaml")
	if nativeSlotSyncOn(t, objects, "pgedge-n1") {
		t.Error("nativeSlotSync: false must leave native slot sync off even for a Spock 6 / PostgreSQL 18 image")
	}
}

func TestNativeSlotSyncRejectsInvalidValue(t *testing.T) {
	out := renderTemplateExpectError(t, "native-slot-sync-invalid-values.yaml")
	if !strings.Contains(out, "nativeSlotSync") {
		t.Errorf("expected a schema error naming nativeSlotSync, got: %s", out)
	}
}
