//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

func TestNodesAddNode(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-minimal-values.yaml")

	for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		t.Fatalf("init-spock job failed: %v", err)
	}

	// Validation: adding a new node without bootstrap.mode should fail
	t.Run("upgrade_rejects_new_node_without_bootstrap_mode", func(t *testing.T) {
		err := tryUpgradeChart("distributed-add-n3-no-bootstrap-values.yaml")
		if err == nil {
			t.Fatal("expected upgrade to fail for new node without bootstrap.mode")
		}
		if !strings.Contains(err.Error(), "must specify bootstrap.mode") {
			t.Errorf("expected 'must specify bootstrap.mode' error, got: %v", err)
		}
	})

	// Validation: re-bootstrapping an existing node should fail
	t.Run("upgrade_rejects_rebootstrap_existing_node", func(t *testing.T) {
		err := tryUpgradeChart("distributed-rebootstrap-n1-values.yaml")
		if err == nil {
			t.Fatal("expected upgrade to fail for existing node with bootstrap.mode set")
		}
		if !strings.Contains(err.Error(), "cannot be re-bootstrapped") {
			t.Errorf("expected 'cannot be re-bootstrapped' error, got: %v", err)
		}
	})

	_, err := testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_add_node (id int PRIMARY KEY, val text);")
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_add_node VALUES (1, 'before-n3');")
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	upgradeChart(t, "distributed-add-n3-values.yaml")

	t.Run("n3_cluster_healthy", func(t *testing.T) {
		if err := wait.ForClusterHealthy(testKube, "pgedge-n3", timeout); err != nil {
			t.Fatalf("cluster pgedge-n3 not healthy: %v", err)
		}
	})

	t.Run("init_spock_succeeds_after_upgrade", func(t *testing.T) {
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed after upgrade: %v\nlogs:\n%s", err, logs)
		}
	})

	t.Run("n3_has_existing_data", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n3-1",
				"SELECT val FROM test_add_node WHERE id = 1;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "before-n3"), nil
		})
		if err != nil {
			t.Error("existing data did not replicate to n3")
		}
	})

	t.Run("full_mesh_replication", func(t *testing.T) {
		// Insert a row on each node and verify it reaches all others
		writes := map[string]int{
			"pgedge-n1-1": 10,
			"pgedge-n2-1": 11,
			"pgedge-n3-1": 12,
		}
		for pod, id := range writes {
			_, err := testKube.ExecSQL(pod,
				fmt.Sprintf("INSERT INTO test_add_node VALUES (%d, 'from-%s');", id, pod))
			if err != nil {
				t.Fatalf("failed to insert on %s: %v", pod, err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		for srcPod, id := range writes {
			for _, dstPod := range []string{"pgedge-n1-1", "pgedge-n2-1", "pgedge-n3-1"} {
				if dstPod == srcPod {
					continue
				}
				t.Run(fmt.Sprintf("%s_to_%s", srcPod, dstPod), func(t *testing.T) {
					expected := fmt.Sprintf("from-%s", srcPod)
					err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
						out, err := testKube.ExecSQL(dstPod,
							fmt.Sprintf("SELECT val FROM test_add_node WHERE id = %d;", id))
						if err != nil {
							return false, nil
						}
						return strings.Contains(out, expected), nil
					})
					if err != nil {
						t.Errorf("data from %s (id=%d) did not replicate to %s", srcPod, id, dstPod)
					}
				})
			}
		}
	})

	t.Run("idempotent_rerun_on_3_nodes", func(t *testing.T) {
		// Record subscription IDs before re-run
		subsBefore := map[string]string{}
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1", "pgedge-n3-1"} {
			out, err := testKube.ExecSQL(pod,
				"SELECT sub_id, sub_name FROM spock.subscription ORDER BY sub_name;")
			if err != nil {
				t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
			}
			subsBefore[pod] = strings.TrimSpace(out)
		}

		// Re-run init-spock on the 3-node cluster (steady-state values, no bootstrap)
		upgradeChart(t, "distributed-3node-values.yaml")

		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock re-run failed: %v\nlogs:\n%s", err, logs)
		}

		// Verify subscriptions were not recreated
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1", "pgedge-n3-1"} {
			out, err := testKube.ExecSQL(pod,
				"SELECT sub_id, sub_name FROM spock.subscription ORDER BY sub_name;")
			if err != nil {
				t.Fatalf("failed to query subscriptions on %s after re-run: %v", pod, err)
			}
			if strings.TrimSpace(out) != subsBefore[pod] {
				t.Errorf("%s: subscriptions changed after re-run\nbefore: %s\nafter:  %s",
					pod, subsBefore[pod], strings.TrimSpace(out))
			}
		}

		// Verify replication still works
		_, err := testKube.ExecSQL("pgedge-n3-1",
			"INSERT INTO test_add_node VALUES (20, 'after-rerun');")
		if err != nil {
			t.Fatalf("failed to insert after re-run: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n1-1",
				"SELECT val FROM test_add_node WHERE id = 20;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-rerun"), nil
		})
		if err != nil {
			t.Error("replication broken after idempotent re-run")
		}
	})
}

func TestNodesRemoveNode(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-3node-values.yaml")

	for _, name := range []string{"pgedge-n1", "pgedge-n2", "pgedge-n3"} {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		t.Fatalf("init-spock job failed: %v", err)
	}

	upgradeChart(t, "distributed-minimal-values.yaml")

	t.Run("remaining_clusters_healthy", func(t *testing.T) {
		for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
			if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
				t.Errorf("cluster %s not healthy after removal: %v", name, err)
			}
		}
	})

	t.Run("init_spock_succeeds_after_removal", func(t *testing.T) {
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed after removal: %v\nlogs:\n%s", err, logs)
		}
	})

	t.Run("spock_node_n3_removed", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT node_name FROM spock.node ORDER BY node_name;")
				if err != nil {
					t.Fatalf("failed to query spock.node on %s: %v", pod, err)
				}
				if strings.Contains(out, "n3") {
					t.Errorf("pod %s: spock.node still contains removed node n3, got: %s", pod, out)
				}
				for _, expected := range []string{"n1", "n2"} {
					if !strings.Contains(out, expected) {
						t.Errorf("pod %s: spock.node missing %s, got: %s", pod, expected, out)
					}
				}
			})
		}
	})

	t.Run("subscriptions_to_n3_removed", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT count(*) FROM spock.subscription;")
				if err != nil {
					t.Fatalf("failed to query subscription count on %s: %v", pod, err)
				}
				if strings.TrimSpace(out) != "1" {
					t.Errorf("pod %s: expected 1 subscription after removing n3, got: %s", pod, out)
				}
			})
		}
	})

	t.Run("replication_still_works", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"CREATE TABLE test_remove (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
		_, err = testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_remove VALUES (1, 'after-remove');")
		if err != nil {
			t.Fatalf("failed to insert: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_remove WHERE id = 1;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-remove"), nil
		})
		if err != nil {
			t.Error("replication between n1 and n2 broken after removing n3")
		}
	})
}
