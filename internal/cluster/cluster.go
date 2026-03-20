// internal/cluster/cluster.go
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var cnpgGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "clusters",
}

const healthyPhase = "Cluster in healthy state"

// getClusters returns sorted CNPG cluster names matching the app label.
func getClusters(ctx context.Context, client dynamic.Interface, namespace, appName string) ([]string, error) {
	list, err := client.Resource(cnpgGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("pgedge.com/app-name=%s", appName),
	})
	if err != nil {
		return nil, fmt.Errorf("list CNPG clusters: %w", err)
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	sort.Strings(names)
	return names, nil
}

// waitForAll polls until all CNPG clusters with the app label are healthy.
func waitForAll(ctx context.Context, client dynamic.Interface, namespace, appName string) error {
	for {
		list, err := client.Resource(cnpgGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("pgedge.com/app-name=%s", appName),
		})
		if err != nil {
			slog.Warn("failed to list clusters", "error", err)
		} else if len(list.Items) > 0 {
			allHealthy := true
			for _, item := range list.Items {
				phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
				if phase != healthyPhase {
					slog.Info("waiting for cluster", "name", item.GetName(), "phase", phase)
					allHealthy = false
				}
			}
			if allHealthy {
				slog.Info("all CNPG clusters healthy", "count", len(list.Items))
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for CNPG clusters: %w", ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
}

// WaitForAll creates an in-cluster K8s client and waits for all CNPG clusters.
func WaitForAll(ctx context.Context, namespace, appName string) error {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("k8s in-cluster config: %w", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("k8s dynamic client: %w", err)
	}
	return waitForAll(ctx, client, namespace, appName)
}
