//go:build unit

package unit

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestInitSpockJobGenerated(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	job := &jobs[0]
	if job.GetName() != "pgedge-init-spock" {
		t.Errorf("expected job name pgedge-init-spock, got %q", job.GetName())
	}

	// Verify hook annotations
	annotations := job.GetAnnotations()
	if annotations["helm.sh/hook"] != "post-install,post-upgrade" {
		t.Errorf("expected post-install,post-upgrade hook, got %q", annotations["helm.sh/hook"])
	}
}

func TestInitSpockJobSkippedWhenDisabled(t *testing.T) {
	objects := renderTemplate(t, "no-init-spock-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 0 {
		t.Errorf("expected 0 Jobs when initSpock disabled, got %d", len(jobs))
	}
}

func TestInitSpockJobDefaultImage(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	containers, found, _ := unstructured.NestedSlice(jobs[0].Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	image, _, _ := unstructured.NestedString(container, "image")
	expected := "ghcr.io/pgedge/pgedge-helm-utils:v0.2.0"
	if image != expected {
		t.Errorf("expected default image %q, got %q", expected, image)
	}
}

func TestInitSpockJobCustomImage(t *testing.T) {
	objects := renderTemplate(t, "custom-image-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	containers, found, _ := unstructured.NestedSlice(jobs[0].Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	image, _, _ := unstructured.NestedString(container, "image")
	expected := "my-registry.io/custom-image:v1.0.0"
	if image != expected {
		t.Errorf("expected custom image %q, got %q", expected, image)
	}
}

func TestInitSpockJobDefaultDBName(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	containers, found, _ := unstructured.NestedSlice(jobs[0].Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	envVars, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range envVars {
		env := e.(map[string]interface{})
		if env["name"] == "DB_NAME" {
			if env["value"] != "app" {
				t.Errorf("expected DB_NAME=app, got %q", env["value"])
			}
			return
		}
	}
	t.Error("DB_NAME env var not found in job container")
}

func TestInitSpockJobCustomDBName(t *testing.T) {
	objects := renderTemplate(t, "custom-database-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	containers, found, _ := unstructured.NestedSlice(jobs[0].Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	envVars, _, _ := unstructured.NestedSlice(container, "env")

	envMap := map[string]string{}
	for _, e := range envVars {
		env := e.(map[string]interface{})
		if name, ok := env["name"].(string); ok {
			if val, ok := env["value"].(string); ok {
				envMap[name] = val
			}
		}
	}

	if envMap["DB_NAME"] != "mydb" {
		t.Errorf("expected DB_NAME=mydb, got %q", envMap["DB_NAME"])
	}
	if envMap["ADMIN_USER"] != "dbadmin" {
		t.Errorf("expected ADMIN_USER=dbadmin, got %q", envMap["ADMIN_USER"])
	}
}

func TestClusterCustomDatabase(t *testing.T) {
	objects := renderTemplate(t, "custom-database-values.yaml")
	clusters := filterByKind(objects, "Cluster")

	if len(clusters) != 1 {
		t.Fatalf("expected 1 Cluster, got %d", len(clusters))
	}

	cluster := &clusters[0]

	db := getNestedString(cluster, "spec", "bootstrap", "initdb", "database")
	if db != "mydb" {
		t.Errorf("expected bootstrap.initdb.database=mydb, got %q", db)
	}

	owner := getNestedString(cluster, "spec", "bootstrap", "initdb", "owner")
	if owner != "myuser" {
		t.Errorf("expected bootstrap.initdb.owner=myuser, got %q", owner)
	}

	hba, found, _ := unstructured.NestedStringSlice(cluster.Object, "spec", "postgresql", "pg_hba")
	if !found {
		t.Fatal("expected pg_hba to be set")
	}
	if len(hba) < 3 {
		t.Fatalf("expected at least 3 pg_hba rules, got %d", len(hba))
	}
	for _, rule := range hba[:3] {
		if !strings.Contains(rule, "mydb") {
			t.Errorf("expected pg_hba rule to reference mydb database, got: %s", rule)
		}
	}
}

func TestCustomCertCommonNames(t *testing.T) {
	objects := renderTemplate(t, "custom-database-values.yaml")
	certs := filterByKind(objects, "Certificate")

	certCN := map[string]string{}
	for _, cert := range certs {
		name := cert.GetName()
		cn := getNestedString(&cert, "spec", "commonName")
		certCN[name] = cn
	}

	if certCN["admin-client-cert"] != "dbadmin" {
		t.Errorf("expected admin-client-cert commonName=dbadmin, got %q", certCN["admin-client-cert"])
	}
	if certCN["app-client-cert"] != "myuser" {
		t.Errorf("expected app-client-cert commonName=myuser, got %q", certCN["app-client-cert"])
	}
}

func TestInitSpockJobDefaultSecurityContext(t *testing.T) {
	objects := renderTemplate(t, "single-node-minimal-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	job := &jobs[0]

	// Pod security context defaults
	runAsNonRoot, found, _ := unstructured.NestedBool(job.Object,
		"spec", "template", "spec", "securityContext", "runAsNonRoot")
	if !found || !runAsNonRoot {
		t.Errorf("expected pod runAsNonRoot=true, got %v (found=%v)", runAsNonRoot, found)
	}
	fsGroup, found, _ := unstructured.NestedInt64(job.Object,
		"spec", "template", "spec", "securityContext", "fsGroup")
	if !found || fsGroup != 65532 {
		t.Errorf("expected pod fsGroup=65532, got %d (found=%v)", fsGroup, found)
	}
	seccompType, found, _ := unstructured.NestedString(job.Object,
		"spec", "template", "spec", "securityContext", "seccompProfile", "type")
	if !found || seccompType != "RuntimeDefault" {
		t.Errorf("expected seccompProfile.type=RuntimeDefault, got %q (found=%v)", seccompType, found)
	}

	// Container security context defaults
	containers, found, _ := unstructured.NestedSlice(job.Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	allowEscalation, found, _ := unstructured.NestedBool(container, "securityContext", "allowPrivilegeEscalation")
	if !found || allowEscalation != false {
		t.Errorf("expected allowPrivilegeEscalation=false, got %v (found=%v)", allowEscalation, found)
	}
	readOnly, found, _ := unstructured.NestedBool(container, "securityContext", "readOnlyRootFilesystem")
	if !found || !readOnly {
		t.Errorf("expected readOnlyRootFilesystem=true, got %v (found=%v)", readOnly, found)
	}
	capDrop, found, _ := unstructured.NestedStringSlice(container, "securityContext", "capabilities", "drop")
	if !found || len(capDrop) != 1 || capDrop[0] != "ALL" {
		t.Errorf("expected capabilities.drop=[ALL], got %v (found=%v)", capDrop, found)
	}
}

func TestSecurityContextCustomOverride(t *testing.T) {
	objects := renderTemplate(t, "custom-security-values.yaml")
	jobs := filterByKind(objects, "Job")

	if len(jobs) != 1 {
		t.Fatalf("expected 1 Job, got %d", len(jobs))
	}

	job := &jobs[0]

	// Check pod security context
	fsGroup, found, _ := unstructured.NestedInt64(job.Object,
		"spec", "template", "spec", "securityContext", "fsGroup")
	if !found || fsGroup != 1000 {
		t.Errorf("expected pod fsGroup=1000, got %d (found=%v)", fsGroup, found)
	}

	// Check container security context
	containers, found, _ := unstructured.NestedSlice(job.Object,
		"spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		t.Fatal("no containers found in job")
	}
	container := containers[0].(map[string]interface{})
	readOnly, found, _ := unstructured.NestedBool(container, "securityContext", "readOnlyRootFilesystem")
	if !found || readOnly != false {
		t.Errorf("expected readOnlyRootFilesystem=false, got %v (found=%v)", readOnly, found)
	}
}
