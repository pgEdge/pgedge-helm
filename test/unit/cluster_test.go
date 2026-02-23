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
	}
	for key, want := range params {
		got := getNestedString(cluster, "spec", "postgresql", "parameters", key)
		if got != want {
			t.Errorf("expected %s=%q, got %q", key, want, got)
		}
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
	expected := "ghcr.io/pgedge/pgedge-postgres:17-spock5-standard"
	if imageName != expected {
		t.Errorf("expected imageName=%q, got %q", expected, imageName)
	}
}
