//go:build unit

package unit

import (
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
