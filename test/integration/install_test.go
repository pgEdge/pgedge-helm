//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

func TestDistributedInstall(t *testing.T) {
	installChart(t, "values-distributed-minimal.yaml")
	t.Cleanup(func() { uninstallChart(t) })

	clusterNames := []string{"pgedge-n1", "pgedge-n2"}

	t.Run("clusters_healthy", func(t *testing.T) {
		for _, name := range clusterNames {
			if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
				t.Errorf("cluster %s not healthy: %v", name, err)
			}
		}
	})

	t.Run("init_spock_job_succeeds", func(t *testing.T) {
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
		}
	})

	t.Run("certificates_ready", func(t *testing.T) {
		certs := []string{
			"client-ca",
			"streaming-replica-client-cert",
			"pgedge-client-cert",
			"admin-client-cert",
			"app-client-cert",
		}
		for _, cert := range certs {
			if err := wait.ForCertReady(testKube, cert, timeout); err != nil {
				t.Errorf("certificate %s not ready: %v", cert, err)
			}
		}
	})

	t.Run("admin_can_connect", func(t *testing.T) {
		out, err := testKube.ConnectWithCert("pgedge-n1-rw", "admin-client-cert", "admin", "app", "SELECT current_user;")
		if err != nil {
			t.Fatalf("admin cert auth connection failed: %v", err)
		}
		if !strings.Contains(out, "admin") {
			t.Errorf("expected current_user=admin, got: %s", out)
		}
	})

	t.Run("app_can_connect", func(t *testing.T) {
		out, err := testKube.ConnectWithCert("pgedge-n1-rw", "app-client-cert", "app", "app", "SELECT current_user;")
		if err != nil {
			t.Fatalf("app cert auth connection failed: %v", err)
		}
		if !strings.Contains(out, "app") {
			t.Errorf("expected current_user=app, got: %s", out)
		}
	})

	t.Run("pgedge_replication_user_exists", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT rolsuper, rolreplication FROM pg_roles WHERE rolname = 'pgedge';")
				if err != nil {
					t.Fatalf("failed to query pg_roles on %s: %v", pod, err)
				}
				if !strings.Contains(out, "t|t") {
					t.Errorf("pod %s: expected pgedge role with superuser+replication, got: %s", pod, out)
				}
			})
		}
	})

	t.Run("spock_nodes_exist", func(t *testing.T) {
		pods := []string{"pgedge-n1-1", "pgedge-n2-1"}
		for _, pod := range pods {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod, "SELECT node_name FROM spock.node ORDER BY node_name;")
				if err != nil {
					t.Fatalf("failed to query spock.node on %s: %v", pod, err)
				}
				for _, expected := range []string{"n1", "n2"} {
					if !strings.Contains(out, expected) {
						t.Errorf("pod %s: spock.node missing %s, got: %s", pod, expected, out)
					}
				}
			})
		}
	})

	t.Run("spock_node_dsn_uses_hostname", func(t *testing.T) {
		expected := map[string]string{
			"n1": "pgedge-n1-rw",
			"n2": "pgedge-n2-rw",
		}
		pod := "pgedge-n1-1"
		for node, hostname := range expected {
			t.Run(node, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT if_dsn FROM spock.node_interface WHERE if_nodeid = (SELECT node_id FROM spock.node WHERE node_name = '"+node+"');")
				if err != nil {
					t.Fatalf("failed to query node interface DSN for %s: %v", node, err)
				}
				if !strings.Contains(out, "host="+hostname) {
					t.Errorf("expected DSN to contain host=%s, got: %s", hostname, out)
				}
			})
		}
	})

	t.Run("subscription_count_and_direction", func(t *testing.T) {
		// 2-node cluster: each node should have exactly 1 subscription (to its peer)
		for _, tc := range []struct {
			pod  string
			peer string
		}{
			{"pgedge-n1-1", "n2"},
			{"pgedge-n2-1", "n1"},
		} {
			t.Run(tc.pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(tc.pod,
					"SELECT count(*) FROM spock.subscription;")
				if err != nil {
					t.Fatalf("failed to query subscription count on %s: %v", tc.pod, err)
				}
				if strings.TrimSpace(out) != "1" {
					t.Errorf("pod %s: expected 1 subscription, got: %s", tc.pod, out)
				}

				out, err = testKube.ExecSQL(tc.pod,
					"SELECT sub_enabled FROM spock.subscription WHERE sub_name LIKE '%"+tc.peer+"%';")
				if err != nil {
					t.Fatalf("failed to query subscription for peer %s on %s: %v", tc.peer, tc.pod, err)
				}
				if !strings.Contains(out, "t") {
					t.Errorf("pod %s: expected enabled subscription to %s, got: %s", tc.pod, tc.peer, out)
				}
			})
		}
	})

	t.Run("data_replicates", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"CREATE TABLE test_repl (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}

		_, err = testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_repl VALUES (1, 'from-n1');")
		if err != nil {
			t.Fatalf("failed to insert on n1: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		t.Run("replicate_to_n2", func(t *testing.T) {
			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := testKube.ExecSQL("pgedge-n2-1", "SELECT val FROM test_repl WHERE id = 1;")
				if err != nil {
					return false, nil
				}
				return strings.Contains(out, "from-n1"), nil
			})
			if err != nil {
				t.Error("row did not replicate to n2 within timeout")
			}
		})

		_, err = testKube.ExecSQL("pgedge-n2-1",
			"INSERT INTO test_repl VALUES (2, 'from-n2');")
		if err != nil {
			t.Fatalf("failed to insert on n2: %v", err)
		}

		t.Run("active_active_n1", func(t *testing.T) {
			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := testKube.ExecSQL("pgedge-n1-1", "SELECT val FROM test_repl WHERE id = 2;")
				if err != nil {
					return false, nil
				}
				return strings.Contains(out, "from-n2"), nil
			})
			if err != nil {
				t.Error("active-active row did not replicate to n1")
			}
		})
	})

	t.Run("idempotent_upgrade", func(t *testing.T) {
		upgradeChart(t, "values-distributed-minimal.yaml")

		t.Run("clusters_healthy", func(t *testing.T) {
			for _, name := range clusterNames {
				if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
					t.Errorf("cluster %s not healthy after upgrade: %v", name, err)
				}
			}
		})

		t.Run("init_spock_succeeds", func(t *testing.T) {
			if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
				logs, _ := testKube.Logs("job/pgedge-init-spock")
				t.Fatalf("init-spock job failed after upgrade: %v\nlogs:\n%s", err, logs)
			}
		})

		t.Run("replication_still_works", func(t *testing.T) {
			_, err := testKube.ExecSQL("pgedge-n1-1",
				"INSERT INTO test_repl VALUES (3, 'after-upgrade');")
			if err != nil {
				t.Fatalf("failed to insert after upgrade: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := testKube.ExecSQL("pgedge-n2-1",
					"SELECT val FROM test_repl WHERE id = 3;")
				if err != nil {
					return false, nil
				}
				return strings.Contains(out, "after-upgrade"), nil
			})
			if err != nil {
				t.Error("replication broken after idempotent upgrade")
			}
		})
	})
}

func TestSingleNodeInstall(t *testing.T) {
	installChart(t, "values-single-node-minimal.yaml")
	t.Cleanup(func() { uninstallChart(t) })

	t.Run("cluster_healthy", func(t *testing.T) {
		if err := wait.ForClusterHealthy(testKube, "pgedge-n1", timeout); err != nil {
			t.Fatalf("cluster pgedge-n1 not healthy: %v", err)
		}
	})

	t.Run("init_spock_job_succeeds", func(t *testing.T) {
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
		}
	})

	t.Run("spock_node_registered", func(t *testing.T) {
		pod := getPodName("pgedge-n1")
		out, err := testKube.ExecSQL(pod, "SELECT node_name FROM spock.node;")
		if err != nil {
			t.Fatalf("failed to query spock.node: %v", err)
		}
		if !strings.Contains(out, "n1") {
			t.Errorf("expected spock node n1 registered, got: %s", out)
		}
	})

	t.Run("no_subscriptions", func(t *testing.T) {
		pod := getPodName("pgedge-n1")
		out, err := testKube.ExecSQL(pod, "SELECT count(*) FROM spock.subscription;")
		if err != nil {
			t.Fatalf("failed to query subscription count: %v", err)
		}
		if strings.TrimSpace(out) != "0" {
			t.Errorf("expected 0 subscriptions for single node, got: %s", out)
		}
	})

	t.Run("certificates_ready", func(t *testing.T) {
		certs := []string{
			"client-ca",
			"streaming-replica-client-cert",
			"pgedge-client-cert",
			"admin-client-cert",
			"app-client-cert",
		}
		for _, cert := range certs {
			if err := wait.ForCertReady(testKube, cert, timeout); err != nil {
				t.Errorf("certificate %s not ready: %v", cert, err)
			}
		}
	})

	t.Run("admin_can_connect", func(t *testing.T) {
		out, err := testKube.ConnectWithCert("pgedge-n1-rw", "admin-client-cert", "admin", "app", "SELECT current_user;")
		if err != nil {
			t.Fatalf("admin cert auth connection failed: %v", err)
		}
		if !strings.Contains(out, "admin") {
			t.Errorf("expected current_user=admin, got: %s", out)
		}
	})
}
