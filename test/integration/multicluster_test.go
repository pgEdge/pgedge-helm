//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/helm"
	"github.com/pgEdge/pgedge-helm/test/pkg/kube"
	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

const (
	nsClusterA = "mc-cluster-a"
	nsClusterB = "mc-cluster-b"
)

// copySecret copies a Kubernetes secret from one namespace to another,
// stripping metadata that would prevent re-creation.
func copySecret(t *testing.T, src, dst *kube.Kubectl, secretName, dstNamespace string) {
	t.Helper()
	raw, err := src.GetJSON("secret", secretName)
	if err != nil {
		t.Fatalf("get secret %s from source: %v", secretName, err)
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal secret %s: %v", secretName, err)
	}

	// Strip metadata fields that prevent re-creation
	if meta, ok := obj["metadata"].(map[string]any); ok {
		meta["namespace"] = dstNamespace
		delete(meta, "uid")
		delete(meta, "resourceVersion")
		delete(meta, "creationTimestamp")
		delete(meta, "managedFields")
		// Remove cert-manager annotations that would confuse cert-manager in dst
		if annotations, ok := meta["annotations"].(map[string]any); ok {
			for k := range annotations {
				if strings.HasPrefix(k, "cert-manager.io/") {
					delete(annotations, k)
				}
			}
		}
		// Remove ownerReferences (cert-manager Certificate owns the secret)
		delete(meta, "ownerReferences")
	}

	cleaned, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal cleaned secret %s: %v", secretName, err)
	}

	if err := dst.Apply(string(cleaned)); err != nil {
		t.Fatalf("apply secret %s to %s: %v", secretName, dstNamespace, err)
	}
}

func TestMultiClusterInstall(t *testing.T) {
	kubeA := &kube.Kubectl{Context: kubeContext, Namespace: nsClusterA}
	kubeB := &kube.Kubectl{Context: kubeContext, Namespace: nsClusterB}
	helmA := &helm.Helm{KubeContext: kubeContext, Namespace: nsClusterA}
	helmB := &helm.Helm{KubeContext: kubeContext, Namespace: nsClusterB}

	t.Cleanup(func() {
		_ = helmB.Uninstall(helmRelease)
		_ = helmA.Uninstall(helmRelease)
		_ = kubeB.DeleteNamespace(nsClusterB)
		_ = kubeA.DeleteNamespace(nsClusterA)
	})

	// Step 1: Deploy cluster A (provisionCerts: true, initSpock: false)
	opts := helm.InstallOpts{
		ChartRef:        chartRef,
		Version:         chartVersion,
		ValuesFiles:     []string{testdataPath("multicluster-a-values.yaml")},
		Wait:            true,
		Timeout:         timeout.String(),
		CreateNamespace: true,
	}
	if initSpockImg != "" {
		opts.SetValues = []string{fmt.Sprintf("pgEdge.initSpockImageName=%s", initSpockImg)}
	}
	if err := helmA.Install(helmRelease, opts); err != nil {
		t.Fatalf("helm install cluster-a failed: %v", err)
	}

	t.Run("cluster_a_healthy", func(t *testing.T) {
		if err := wait.ForClusterHealthy(kubeA, "pgedge-n1", timeout); err != nil {
			t.Fatalf("cluster pgedge-n1 not healthy: %v", err)
		}
	})

	// Step 2: Wait for certs to be issued in cluster A
	t.Run("cluster_a_certs_ready", func(t *testing.T) {
		certs := []string{
			"client-ca",
			"streaming-replica-client-cert",
			"pgedge-client-cert",
			"admin-client-cert",
			"app-client-cert",
		}
		for _, cert := range certs {
			if err := wait.ForCertReady(kubeA, cert, timeout); err != nil {
				t.Fatalf("certificate %s not ready in cluster-a: %v", cert, err)
			}
		}
	})

	// Step 3: Copy cert secrets from cluster A → cluster B
	t.Run("copy_certs_to_cluster_b", func(t *testing.T) {
		if err := kubeA.CreateNamespace(nsClusterB); err != nil {
			t.Fatalf("create namespace %s: %v", nsClusterB, err)
		}

		secrets := []string{
			"client-ca-key-pair",
			"streaming-replica-client-cert",
			"pgedge-client-cert",
			"admin-client-cert",
			"app-client-cert",
		}
		for _, s := range secrets {
			copySecret(t, kubeA, kubeB, s, nsClusterB)
		}
	})

	// Step 4: Deploy cluster B (provisionCerts: false, initSpock: true)
	optsB := helm.InstallOpts{
		ChartRef:    chartRef,
		Version:     chartVersion,
		ValuesFiles: []string{testdataPath("multicluster-b-values.yaml")},
		Wait:        true,
		Timeout:     timeout.String(),
	}
	if initSpockImg != "" {
		optsB.SetValues = []string{fmt.Sprintf("pgEdge.initSpockImageName=%s", initSpockImg)}
	}
	if err := helmB.Install(helmRelease, optsB); err != nil {
		t.Fatalf("helm install cluster-b failed: %v", err)
	}

	t.Run("cluster_b_healthy", func(t *testing.T) {
		if err := wait.ForClusterHealthy(kubeB, "pgedge-n2", timeout); err != nil {
			t.Fatalf("cluster pgedge-n2 not healthy: %v", err)
		}
	})

	t.Run("init_spock_job_succeeds", func(t *testing.T) {
		if err := wait.ForJobComplete(kubeB, "pgedge-init-spock", timeout); err != nil {
			logs, _ := kubeB.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
		}
	})

	// Assertions
	t.Run("spock_nodes_on_both", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			kube *kube.Kubectl
			pod  string
		}{
			{"cluster_a", kubeA, "pgedge-n1-1"},
			{"cluster_b", kubeB, "pgedge-n2-1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := tc.kube.ExecSQL(tc.pod, "SELECT node_name FROM spock.node ORDER BY node_name;")
				if err != nil {
					t.Fatalf("failed to query spock.node on %s: %v", tc.pod, err)
				}
				for _, expected := range []string{"n1", "n2"} {
					if !strings.Contains(out, expected) {
						t.Errorf("%s: spock.node missing %s, got: %s", tc.pod, expected, out)
					}
				}
			})
		}
	})

	t.Run("dsn_uses_fqdn", func(t *testing.T) {
		expected := map[string]string{
			"n1": "pgedge-n1-rw.mc-cluster-a.svc.cluster.local",
			"n2": "pgedge-n2-rw.mc-cluster-b.svc.cluster.local",
		}
		for node, hostname := range expected {
			t.Run(node, func(t *testing.T) {
				out, err := kubeA.ExecSQL("pgedge-n1-1",
					"SELECT if_dsn FROM spock.node_interface WHERE if_nodeid = (SELECT node_id FROM spock.node WHERE node_name = '"+node+"');")
				if err != nil {
					t.Fatalf("failed to query DSN for %s: %v", node, err)
				}
				if !strings.Contains(out, "host="+hostname) {
					t.Errorf("expected DSN to contain host=%s, got: %s", hostname, out)
				}
			})
		}
	})

	t.Run("subscriptions_bidirectional", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			kube *kube.Kubectl
			pod  string
		}{
			{"cluster_a", kubeA, "pgedge-n1-1"},
			{"cluster_b", kubeB, "pgedge-n2-1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, err := tc.kube.ExecSQL(tc.pod, "SELECT count(*) FROM spock.subscription;")
				if err != nil {
					t.Fatalf("failed to query subscription count on %s: %v", tc.pod, err)
				}
				if strings.TrimSpace(out) != "1" {
					t.Errorf("%s: expected 1 subscription, got: %s", tc.pod, out)
				}
			})
		}
	})

	t.Run("data_replicates_cross_namespace", func(t *testing.T) {
		_, err := kubeA.ExecSQL("pgedge-n1-1",
			"CREATE TABLE test_mc (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table on n1: %v", err)
		}

		_, err = kubeA.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_mc VALUES (1, 'from-cluster-a');")
		if err != nil {
			t.Fatalf("failed to insert on n1: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		t.Run("a_to_b", func(t *testing.T) {
			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := kubeB.ExecSQL("pgedge-n2-1", "SELECT val FROM test_mc WHERE id = 1;")
				if err != nil {
					return false, nil
				}
				return strings.Contains(out, "from-cluster-a"), nil
			})
			if err != nil {
				t.Error("row did not replicate from cluster-a to cluster-b")
			}
		})

		_, err = kubeB.ExecSQL("pgedge-n2-1",
			"INSERT INTO test_mc VALUES (2, 'from-cluster-b');")
		if err != nil {
			t.Fatalf("failed to insert on n2: %v", err)
		}

		t.Run("b_to_a", func(t *testing.T) {
			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := kubeA.ExecSQL("pgedge-n1-1", "SELECT val FROM test_mc WHERE id = 2;")
				if err != nil {
					return false, nil
				}
				return strings.Contains(out, "from-cluster-b"), nil
			})
			if err != nil {
				t.Error("row did not replicate from cluster-b to cluster-a")
			}
		})
	})
}
