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
