// internal/cluster/cluster_test.go
package cluster

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newCluster(name, namespace, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"pgedge.com/app-name": "pgedge",
				},
			},
			"status": map[string]interface{}{
				"phase": phase,
			},
		},
	}
}

func TestGetClusters(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{cnpgGVR: "ClusterList"},
		newCluster("pgedge-n2", "default", "Cluster in healthy state"),
		newCluster("pgedge-n1", "default", "Cluster in healthy state"),
	)

	names, err := getClusters(context.Background(), client, "default", "pgedge")
	if err != nil {
		t.Fatalf("getClusters: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "pgedge-n1" || names[1] != "pgedge-n2" {
		t.Errorf("expected sorted [pgedge-n1, pgedge-n2], got %v", names)
	}
}

func TestGetClustersEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{cnpgGVR: "ClusterList"},
	)

	names, err := getClusters(context.Background(), client, "default", "pgedge")
	if err != nil {
		t.Fatalf("getClusters: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(names))
	}
}

func TestWaitForAllHealthy(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{cnpgGVR: "ClusterList"},
		newCluster("pgedge-n1", "default", "Cluster in healthy state"),
		newCluster("pgedge-n2", "default", "Cluster in healthy state"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := waitForAll(ctx, client, "default", "pgedge")
	if err != nil {
		t.Fatalf("waitForAll: %v", err)
	}
}

func TestWaitForAllTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{cnpgGVR: "ClusterList"},
		newCluster("pgedge-n1", "default", "Setting up primary"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := waitForAll(ctx, client, "default", "pgedge")
	if err == nil {
		t.Error("expected timeout error for unhealthy cluster")
	}
}

func TestHASpecs(t *testing.T) {
	withSpec := func(name string, spec map[string]interface{}) *unstructured.Unstructured {
		c := newCluster(name, "default", "Cluster in healthy state")
		c.Object["spec"] = spec
		return c
	}
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{cnpgGVR: "ClusterList"},
		withSpec("pgedge-n1", map[string]interface{}{"instances": int64(1)}),
		withSpec("pgedge-n2", map[string]interface{}{
			"instances": int64(3),
			"replicationSlots": map[string]interface{}{
				"highAvailability": map[string]interface{}{
					"enabled":                    true,
					"synchronizeLogicalDecoding": true,
				},
			},
		}),
		withSpec("pgedge-n3", map[string]interface{}{
			"instances": int64(2),
			"replicationSlots": map[string]interface{}{
				"highAvailability": map[string]interface{}{"enabled": false},
			},
		}),
	)

	specs, err := haSpecs(context.Background(), client, "default", "pgedge")
	if err != nil {
		t.Fatalf("haSpecs: %v", err)
	}
	want := map[string]HASpec{
		// HA slots default to enabled when the spec doesn't say.
		"pgedge-n1": {Instances: 1, HASlots: true},
		"pgedge-n2": {Instances: 3, HASlots: true, SynchronizeLogicalDecoding: true},
		"pgedge-n3": {Instances: 2, HASlots: false},
	}
	for name, w := range want {
		if specs[name] != w {
			t.Errorf("%s: got %+v, want %+v", name, specs[name], w)
		}
	}
}
