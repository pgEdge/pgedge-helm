//go:build unit

package unit

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConfigMapContainsNodeConfig(t *testing.T) {
	objects := renderTemplate(t, "distributed-values.yaml")
	cm := findByKindAndName(objects, "ConfigMap", "pgedge-config")

	if cm == nil {
		t.Fatal("pgedge-config ConfigMap not found")
	}

	data, found, _ := unstructured.NestedStringMap(cm.Object, "data")
	if !found {
		t.Fatal("ConfigMap has no data")
	}

	nodes, ok := data["nodes"]
	if !ok {
		t.Fatal("ConfigMap missing 'nodes' key")
	}

	for _, name := range []string{"n1", "n2", "n3"} {
		if !strings.Contains(nodes, name) {
			t.Errorf("ConfigMap nodes data missing node %q", name)
		}
	}
	for _, hostname := range []string{"pgedge-n1-rw", "pgedge-n2-rw", "pgedge-n3-rw"} {
		if !strings.Contains(nodes, hostname) {
			t.Errorf("ConfigMap nodes data missing hostname %q", hostname)
		}
	}
}

func TestConfigMapExternalNodes(t *testing.T) {
	objects := renderTemplate(t, "external-nodes-values.yaml")
	cm := findByKindAndName(objects, "ConfigMap", "pgedge-config")

	if cm == nil {
		t.Fatal("pgedge-config ConfigMap not found")
	}

	data, found, _ := unstructured.NestedStringMap(cm.Object, "data")
	if !found {
		t.Fatal("ConfigMap has no data")
	}

	nodes := data["nodes"]

	// Local nodes
	for _, name := range []string{"n1", "n2"} {
		if !strings.Contains(nodes, name) {
			t.Errorf("ConfigMap nodes missing local node %q", name)
		}
	}

	// External node
	if !strings.Contains(nodes, "n3") {
		t.Error("ConfigMap nodes missing external node n3")
	}
	if !strings.Contains(nodes, "external-n3.example.com") {
		t.Error("ConfigMap nodes missing external hostname external-n3.example.com")
	}
}

func TestConfigMapInternalHostname(t *testing.T) {
	objects := renderTemplate(t, "internal-hostname-values.yaml")
	cm := findByKindAndName(objects, "ConfigMap", "pgedge-config")

	if cm == nil {
		t.Fatal("pgedge-config ConfigMap not found")
	}

	data, found, _ := unstructured.NestedStringMap(cm.Object, "data")
	if !found {
		t.Fatal("ConfigMap has no data")
	}

	nodes := data["nodes"]

	// External hostnames
	if !strings.Contains(nodes, "pgedge-n1.external.example.com") {
		t.Error("ConfigMap missing external hostname pgedge-n1.external.example.com")
	}

	// Internal hostnames
	if !strings.Contains(nodes, "pgedge-n1-rw") {
		t.Error("ConfigMap missing internalHostname pgedge-n1-rw")
	}
	if !strings.Contains(nodes, "pgedge-n2-rw") {
		t.Error("ConfigMap missing internalHostname pgedge-n2-rw")
	}
}
