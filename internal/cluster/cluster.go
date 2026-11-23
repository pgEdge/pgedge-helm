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
		} else if len(list.Items) == 0 {
			slog.Info("no CNPG clusters found yet", "namespace", namespace, "appName", appName)
		} else {
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
	client, err := newClient()
	if err != nil {
		return err
	}
	return waitForAll(ctx, client, namespace, appName)
}

// HASpec is the part of a CNPG Cluster spec that decides whether its
// standbys can keep copies of logical replication slots.
type HASpec struct {
	// Instances is spec.instances.
	Instances int64
	// HASlots is spec.replicationSlots.highAvailability.enabled, which CNPG
	// defaults to true.
	HASlots bool
	// SynchronizeLogicalDecoding is
	// spec.replicationSlots.highAvailability.synchronizeLogicalDecoding.
	// CNPG needs it to add dbname to the standbys' primary_conninfo, which
	// PostgreSQL's native slot sync requires, and to manage
	// synchronized_standby_slots.
	SynchronizeLogicalDecoding bool
}

// haSpecs returns the HASpec of each CNPG cluster with the app label, keyed
// by cluster name.
func haSpecs(ctx context.Context, client dynamic.Interface, namespace, appName string) (map[string]HASpec, error) {
	list, err := client.Resource(cnpgGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("pgedge.com/app-name=%s", appName),
	})
	if err != nil {
		return nil, fmt.Errorf("list CNPG clusters: %w", err)
	}
	specs := make(map[string]HASpec, len(list.Items))
	for _, item := range list.Items {
		instances, _, _ := unstructured.NestedInt64(item.Object, "spec", "instances")
		haSlots, found, _ := unstructured.NestedBool(item.Object,
			"spec", "replicationSlots", "highAvailability", "enabled")
		if !found {
			haSlots = true
		}
		syncLD, _, _ := unstructured.NestedBool(item.Object,
			"spec", "replicationSlots", "highAvailability", "synchronizeLogicalDecoding")
		specs[item.GetName()] = HASpec{
			Instances:                  instances,
			HASlots:                    haSlots,
			SynchronizeLogicalDecoding: syncLD,
		}
	}
	return specs, nil
}

// HASpecs creates an in-cluster K8s client and returns the HASpec of each
// CNPG cluster with the app label, keyed by cluster name.
func HASpecs(ctx context.Context, namespace, appName string) (map[string]HASpec, error) {
	client, err := newClient()
	if err != nil {
		return nil, err
	}
	return haSpecs(ctx, client, namespace, appName)
}

func newClient() (dynamic.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("k8s in-cluster config: %w", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s dynamic client: %w", err)
	}
	return client, nil
}
