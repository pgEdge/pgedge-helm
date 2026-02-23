//go:build unit

package unit

import "testing"

func TestExtraResourcesRendered(t *testing.T) {
	objects := renderTemplate(t, "values-extra-resources.yaml")
	netpols := filterByKind(objects, "NetworkPolicy")

	if len(netpols) != 1 {
		t.Fatalf("expected 1 NetworkPolicy, got %d", len(netpols))
	}
	if netpols[0].GetName() != "pgedge-deny-all" {
		t.Errorf("expected NetworkPolicy name pgedge-deny-all, got %q", netpols[0].GetName())
	}
}

func TestCustomAppNamePropagation(t *testing.T) {
	objects := renderTemplate(t, "values-custom-appname.yaml")

	// Clusters should be named myapp-n1, myapp-n2
	clusters := filterByKind(objects, "Cluster")
	if len(clusters) != 2 {
		t.Fatalf("expected 2 Clusters, got %d", len(clusters))
	}
	for _, name := range []string{"myapp-n1", "myapp-n2"} {
		if findByKindAndName(objects, "Cluster", name) == nil {
			t.Errorf("expected Cluster %q not found", name)
		}
	}

	// Cluster labels
	for _, cluster := range clusters {
		labels := cluster.GetLabels()
		if labels["pgedge.com/app-name"] != "myapp" {
			t.Errorf("%s: expected label pgedge.com/app-name=myapp, got %q",
				cluster.GetName(), labels["pgedge.com/app-name"])
		}
	}

	// ConfigMap named myapp-config
	cm := findByKindAndName(objects, "ConfigMap", "myapp-config")
	if cm == nil {
		t.Error("expected ConfigMap myapp-config not found")
	}

	// Init-spock Job named myapp-init-spock
	job := findByKindAndName(objects, "Job", "myapp-init-spock")
	if job == nil {
		t.Error("expected Job myapp-init-spock not found")
	}

	// RBAC resources
	sa := findByKindAndName(objects, "ServiceAccount", "myapp-init-spock")
	if sa == nil {
		t.Error("expected ServiceAccount myapp-init-spock not found")
	}
	role := findByKindAndName(objects, "Role", "myapp-init-spock")
	if role == nil {
		t.Error("expected Role myapp-init-spock not found")
	}
	rb := findByKindAndName(objects, "RoleBinding", "myapp-init-spock")
	if rb == nil {
		t.Error("expected RoleBinding myapp-init-spock not found")
	}
}
