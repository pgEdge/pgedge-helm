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

func TestNodesAddNodeZeroDowntime(t *testing.T) {
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

	allPods := []string{"pgedge-n1-1", "pgedge-n2-1", "pgedge-n3-1"}
	writerPods := []string{"pgedge-n1-1", "pgedge-n2-1"}

	// Create per-node tables for each writer
	for _, pod := range writerPods {
		node := podToNode(pod)
		_, err := testKube.ExecSQL(pod,
			fmt.Sprintf("CREATE TABLE test_zdt_%s (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), data text);", node))
		if err != nil {
			t.Fatalf("failed to create table on %s: %v", pod, err)
		}
	}

	// Seed initial data
	_, err := testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_zdt_n1 (data) VALUES ('before-upgrade');")
	if err != nil {
		t.Fatalf("failed to insert initial data: %v", err)
	}

	// Start background writers — each writes to its own table
	ctx, cancelWriter := context.WithCancel(context.Background())
	writerDone := make(chan error, len(writerPods))
	for _, pod := range writerPods {
		go func(pod string) {
			node := podToNode(pod)
			table := "test_zdt_" + node
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					writerDone <- nil
					return
				case <-ticker.C:
					_, err := testKube.ExecSQL(pod,
						fmt.Sprintf("INSERT INTO %s (data) SELECT md5(random()::text) FROM generate_series(1, 5);", table))
					if err != nil {
						writerDone <- fmt.Errorf("background write to %s failed: %w", pod, err)
						return
					}
				}
			}
		}(pod)
	}

	// Add n3 while writers are running
	upgradeChart(t, "distributed-add-n3-values.yaml")

	t.Run("n3_cluster_healthy", func(t *testing.T) {
		if err := wait.ForClusterHealthy(testKube, "pgedge-n3", timeout); err != nil {
			t.Fatalf("cluster pgedge-n3 not healthy: %v", err)
		}
	})

	t.Run("init_spock_succeeds", func(t *testing.T) {
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
		}
	})

	// Confirm replication caught up while writers are still running
	waitForReplication(t, allPods)

	// Stop writers
	cancelWriter()
	for range len(writerPods) {
		if err := <-writerDone; err != nil {
			t.Fatalf("background writer failed: %v", err)
		}
	}

	// Confirm replication caught up after final writes
	waitForReplication(t, allPods)

	t.Run("n3_has_all_data", func(t *testing.T) {
		// All replication paths confirmed caught up — counts must match
		for _, table := range []string{"test_zdt_n1", "test_zdt_n2"} {
			t.Run(table, func(t *testing.T) {
				counts := make(map[string]string)
				for _, pod := range allPods {
					cnt, err := testKube.ExecSQL(pod,
						fmt.Sprintf("SELECT count(*) FROM %s;", table))
					if err != nil {
						t.Fatalf("count on %s: %v", pod, err)
					}
					counts[pod] = strings.TrimSpace(cnt)
				}
				if counts["pgedge-n1-1"] != counts["pgedge-n2-1"] || counts["pgedge-n1-1"] != counts["pgedge-n3-1"] {
					t.Errorf("%s count mismatch: n1=%s n2=%s n3=%s",
						table, counts["pgedge-n1-1"], counts["pgedge-n2-1"], counts["pgedge-n3-1"])
				}
			})
		}
	})

	t.Run("n3_replicates_bidirectionally", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n3-1",
			"CREATE TABLE test_zdt_n3 (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), data text);")
		if err != nil {
			t.Fatalf("failed to create table on n3: %v", err)
		}
		_, err = testKube.ExecSQL("pgedge-n3-1",
			"INSERT INTO test_zdt_n3 (data) VALUES ('from-n3');")
		if err != nil {
			t.Fatalf("failed to write on n3: %v", err)
		}

		waitForReplication(t, allPods)

		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT data FROM test_zdt_n3 WHERE data = 'from-n3';")
				if err != nil {
					t.Fatalf("query failed on %s: %v", pod, err)
				}
				if !strings.Contains(out, "from-n3") {
					t.Errorf("data from n3 did not replicate to %s", pod)
				}
			})
		}
	})

	t.Run("full_mesh_established", func(t *testing.T) {
		for _, pod := range allPods {
			t.Run(pod+"_subscriptions", func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT count(*) FROM spock.subscription;")
				if err != nil {
					t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
				}
				if strings.TrimSpace(out) != "2" {
					t.Errorf("%s: expected 2 subscriptions, got %s", pod, strings.TrimSpace(out))
				}
			})
		}
	})
}

// waitForReplication fires a sync event on each pod and waits for every other
// pod to receive it, confirming all replication paths have caught up.
// Matches the Control Plane's WaitForReplication pattern.
func waitForReplication(t *testing.T, pods []string) {
	t.Helper()

	type syncEvt struct {
		provider string
		lsn      string
	}

	var events []syncEvt
	for _, pod := range pods {
		lsn, err := testKube.ExecSQL(pod, "SELECT spock.sync_event();")
		if err != nil {
			t.Fatalf("sync_event on %s: %v", pod, err)
		}
		events = append(events, syncEvt{podToNode(pod), strings.TrimSpace(lsn)})
	}

	for _, evt := range events {
		for _, dst := range pods {
			if podToNode(dst) == evt.provider {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				_, err := testKube.ExecSQL(dst,
					fmt.Sprintf("CALL spock.wait_for_sync_event(true, '%s', '%s'::pg_lsn, 10);",
						evt.provider, evt.lsn))
				return err == nil, nil
			})
			cancel()
			if err != nil {
				t.Fatalf("%s did not receive sync event from %s (lsn=%s)", dst, evt.provider, evt.lsn)
			}
		}
	}
}

// podToNode extracts the node name from a pod name (e.g. "pgedge-n1-1" -> "n1").
func podToNode(pod string) string {
	return strings.TrimSuffix(strings.TrimPrefix(pod, "pgedge-"), "-1")
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
