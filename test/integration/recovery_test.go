//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

// dropSlotsSQL is a PL/pgSQL block that terminates walsenders and drops all
// logical replication slots, retrying until the async termination completes.
var dropSlotsSQL = `DO $body$
DECLARE
  r RECORD;
  attempts INT;
BEGIN
  FOR r IN SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical'
  LOOP
    attempts := 0;
    LOOP
      PERFORM pg_terminate_backend(active_pid)
        FROM pg_replication_slots
        WHERE slot_name = r.slot_name AND active;
      BEGIN
        PERFORM pg_drop_replication_slot(r.slot_name);
        EXIT;
      EXCEPTION WHEN object_in_use THEN
        attempts := attempts + 1;
        IF attempts > 20 THEN
          RAISE EXCEPTION 'failed to drop slot % after 20 attempts', r.slot_name;
        END IF;
        PERFORM pg_sleep(0.5);
      END;
    END LOOP;
  END LOOP;
END $body$;`

// TestSlotLossRecovery simulates a PG upgrade or database restore where
// replication slots are lost but subscriptions remain. Re-running init-spock
// should detect missing slots and recreate subscriptions (which recreates slots).
func TestSlotLossRecovery(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-minimal-values.yaml")

	clusterNames := []string{"pgedge-n1", "pgedge-n2"}
	pods := []string{"pgedge-n1-1", "pgedge-n2-1"}

	for _, name := range clusterNames {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// Verify initial replication works.
	_, err := testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_slot_recovery (id int PRIMARY KEY, val text);")
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_slot_recovery VALUES (1, 'before-slot-drop');")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
		out, err := testKube.ExecSQL("pgedge-n2-1",
			"SELECT val FROM test_slot_recovery WHERE id = 1;")
		if err != nil {
			return false, nil
		}
		return strings.Contains(out, "before-slot-drop"), nil
	})
	if err != nil {
		t.Fatalf("initial replication failed: row did not reach n2")
	}

	// Simulate slot loss: terminate walsenders and drop all logical replication slots.
	// pg_terminate_backend is asynchronous, so we retry the drop in a loop until
	// the walsender has fully exited.
	t.Run("drop_all_slots", func(t *testing.T) {
		for _, pod := range pods {
			_, err := testKube.ExecSQL(pod, dropSlotsSQL)
			if err != nil {
				t.Fatalf("failed to drop replication slots on %s: %v", pod, err)
			}

			// Verify slots are gone.
			out, err := testKube.ExecSQL(pod,
				"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
			if err != nil {
				t.Fatalf("failed to count slots on %s: %v", pod, err)
			}
			if strings.TrimSpace(out) != "0" {
				t.Errorf("pod %s: expected 0 slots after drop, got: %s", pod, out)
			}
		}
	})

	// Verify subscriptions still exist (they survive slot loss).
	t.Run("subscriptions_survive_slot_loss", func(t *testing.T) {
		for _, pod := range pods {
			out, err := testKube.ExecSQL(pod,
				"SELECT count(*) FROM spock.subscription;")
			if err != nil {
				t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
			}
			if strings.TrimSpace(out) != "1" {
				t.Errorf("pod %s: expected 1 subscription to survive, got: %s", pod, out)
			}
		}
	})

	// Re-run init-spock — should detect missing slots and recreate subscriptions.
	t.Run("recovery_via_upgrade", func(t *testing.T) {
		upgradeChart(t, "distributed-minimal-values.yaml")

		for _, name := range clusterNames {
			if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
				t.Fatalf("cluster %s not healthy after recovery: %v", name, err)
			}
		}
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed during recovery: %v\nlogs:\n%s", err, logs)
		}
	})

	// Verify slots are restored.
	t.Run("slots_restored", func(t *testing.T) {
		for _, pod := range pods {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
				out, err := testKube.ExecSQL(pod,
					"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
				if err != nil {
					return false, nil
				}
				return strings.TrimSpace(out) == "1", nil
			})
			if err != nil {
				t.Errorf("pod %s: replication slot not restored after recovery", pod)
			}
		}
	})

	// Verify replication works after recovery.
	t.Run("replication_works_after_recovery", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_slot_recovery VALUES (2, 'after-recovery');")
		if err != nil {
			t.Fatalf("failed to insert after recovery: %v", err)
		}

		ctx1, cancel1 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel1()

		err = wait.Until(ctx1, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_slot_recovery WHERE id = 2;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-recovery"), nil
		})
		if err != nil {
			t.Error("replication broken after slot recovery")
		}

		// Verify active-active: n2 → n1.
		_, err = testKube.ExecSQL("pgedge-n2-1",
			"INSERT INTO test_slot_recovery VALUES (3, 'n2-after-recovery');")
		if err != nil {
			t.Fatalf("failed to insert on n2 after recovery: %v", err)
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel2()

		err = wait.Until(ctx2, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n1-1",
				"SELECT val FROM test_slot_recovery WHERE id = 3;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "n2-after-recovery"), nil
		})
		if err != nil {
			t.Error("active-active replication broken after slot recovery")
		}
	})
}

// TestOrphanSlotCleanupAfterNodeRemoval verifies that orphan replication slots
// on surviving nodes are cleaned up when a node is removed from the config.
func TestOrphanSlotCleanupAfterNodeRemoval(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-3node-values.yaml")

	allClusters := []string{"pgedge-n1", "pgedge-n2", "pgedge-n3"}
	for _, name := range allClusters {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// Verify 3-node mesh: each node should have 2 logical replication slots.
	t.Run("initial_slot_count", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1", "pgedge-n3-1"} {
			out, err := testKube.ExecSQL(pod,
				"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
			if err != nil {
				t.Fatalf("failed to query slots on %s: %v", pod, err)
			}
			if strings.TrimSpace(out) != "2" {
				t.Errorf("pod %s: expected 2 slots in 3-node mesh, got: %s", pod, out)
			}
		}
	})

	// Remove n3: downgrade to 2-node config.
	upgradeChart(t, "distributed-minimal-values.yaml")

	for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy after removal: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed after removal: %v\nlogs:\n%s", err, logs)
	}

	// Verify orphan slots from n3 are cleaned up.
	t.Run("orphan_slots_cleaned", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()

				err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
					out, err := testKube.ExecSQL(pod,
						"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
					if err != nil {
						return false, nil
					}
					return strings.TrimSpace(out) == "1", nil
				})
				if err != nil {
					// Log the actual slot names for debugging.
					out, _ := testKube.ExecSQL(pod,
						"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical';")
					t.Errorf("pod %s: expected 1 slot after removing n3, remaining slots: %s", pod, out)
				}
			})
		}
	})

	// Verify no slot names reference n3.
	t.Run("no_n3_slot_references", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical';")
				if err != nil {
					t.Fatalf("failed to query slot names on %s: %v", pod, err)
				}
				if strings.Contains(out, "n3") {
					t.Errorf("pod %s: found slot referencing removed node n3: %s", pod, out)
				}
			})
		}
	})

	// Verify replication still works between surviving nodes.
	t.Run("replication_works_after_cleanup", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"CREATE TABLE test_orphan_cleanup (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
		_, err = testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_orphan_cleanup VALUES (1, 'after-cleanup');")
		if err != nil {
			t.Fatalf("failed to insert: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_orphan_cleanup WHERE id = 1;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-cleanup"), nil
		})
		if err != nil {
			t.Error("replication broken between surviving nodes after orphan cleanup")
		}
	})
}

// TestCNPGBootstrapRecovery simulates a cross-node CNPG restore where n2 is
// restored from n1's backup. After restore, n2's spock.node_local returns "n1"
// instead of "n2". Init-spock should detect the mismatch, run deleteAll (backup
// repsets, drop all subs/nodes), then recreate everything on n2.
func TestCNPGBootstrapRecovery(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-minimal-values.yaml")

	clusterNames := []string{"pgedge-n1", "pgedge-n2"}

	for _, name := range clusterNames {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// Verify initial replication works.
	_, err := testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_bootstrap (id int PRIMARY KEY, val text);")
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_bootstrap VALUES (1, 'before-restore');")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
		out, err := testKube.ExecSQL("pgedge-n2-1",
			"SELECT val FROM test_bootstrap WHERE id = 1;")
		if err != nil {
			return false, nil
		}
		return strings.Contains(out, "before-restore"), nil
	})
	if err != nil {
		t.Fatalf("initial replication failed: row did not reach n2")
	}

	// Simulate cross-node restore on n2. A real pg_basebackup restore would:
	// 1. Copy n1's data (including spock metadata) to n2
	// 2. NOT copy replication slots (they're instance-level state)
	// So n2 ends up with n1's spock identity and no replication slots.
	t.Run("simulate_restore", func(t *testing.T) {
		// Drop replication slots on n2 (pg_basebackup doesn't copy them).
		_, err := testKube.ExecSQL("pgedge-n2-1", dropSlotsSQL)
		if err != nil {
			t.Fatalf("failed to drop slots on n2: %v", err)
		}

		// Swap local_node to point to n1's identity.
		_, err = testKube.ExecSQL("pgedge-n2-1", `DO $body$
DECLARE
  remote_node_id oid;
  remote_if_id oid;
  local_name text;
BEGIN
  PERFORM spock.repair_mode('True');

  -- Find n1's node_id and interface_id (the "source" of the restore)
  SELECT n.node_id, ni.if_id INTO remote_node_id, remote_if_id
    FROM spock.node n
    JOIN spock.node_interface ni ON ni.if_nodeid = n.node_id
    WHERE n.node_id NOT IN (SELECT node_id FROM spock.local_node);

  -- Point local_node to n1's node AND interface (simulates restore from n1 backup)
  UPDATE spock.local_node SET node_id = remote_node_id, node_local_interface = remote_if_id;

  -- Verify: local node should now resolve to 'n1'
  SELECT n.node_name INTO local_name
    FROM spock.node n JOIN spock.local_node ln ON n.node_id = ln.node_id;
  IF local_name != 'n1' THEN
    RAISE EXCEPTION 'simulation failed: local node should be n1, got %', local_name;
  END IF;
END $body$;`)
		if err != nil {
			t.Fatalf("failed to simulate restore on n2: %v", err)
		}

		// Verify the simulation worked.
		out, err := testKube.ExecSQL("pgedge-n2-1",
			"SELECT n.node_name FROM spock.node n JOIN spock.local_node ln ON n.node_id = ln.node_id;")
		if err != nil {
			t.Fatalf("failed to query local node: %v", err)
		}
		if strings.TrimSpace(out) != "n1" {
			t.Fatalf("expected local node=n1 after simulation, got: %s", out)
		}
	})

	// Re-run init-spock — should detect node_local mismatch and run deleteAll.
	t.Run("recovery_via_upgrade", func(t *testing.T) {
		upgradeChart(t, "distributed-minimal-values.yaml")

		for _, name := range clusterNames {
			if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
				t.Fatalf("cluster %s not healthy after recovery: %v", name, err)
			}
		}
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed during recovery: %v\nlogs:\n%s", err, logs)
		}
	})

	// Verify n2's spock identity is correct after recovery.
	t.Run("node_identity_restored", func(t *testing.T) {
		out, err := testKube.ExecSQL("pgedge-n2-1",
			"SELECT n.node_name FROM spock.node n JOIN spock.local_node ln ON n.node_id = ln.node_id;")
		if err != nil {
			t.Fatalf("failed to query local node: %v", err)
		}
		if strings.TrimSpace(out) != "n2" {
			t.Errorf("expected local node=n2 after recovery, got: %s", out)
		}
	})

	// Verify both nodes see both spock nodes.
	t.Run("spock_nodes_correct", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT node_name FROM spock.node ORDER BY node_name;")
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

	// Verify subscriptions are re-established.
	t.Run("subscriptions_reestablished", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT count(*) FROM spock.subscription;")
				if err != nil {
					t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
				}
				if strings.TrimSpace(out) != "1" {
					t.Errorf("pod %s: expected 1 subscription, got: %s", pod, out)
				}
			})
		}
	})

	// Verify replication slots are back.
	t.Run("slots_restored", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()

				err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
					out, err := testKube.ExecSQL(pod,
						"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
					if err != nil {
						return false, nil
					}
					return strings.TrimSpace(out) == "1", nil
				})
				if err != nil {
					t.Errorf("pod %s: replication slot not restored", pod)
				}
			})
		}
	})

	// Verify replication works end-to-end after recovery.
	t.Run("replication_works_after_recovery", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_bootstrap VALUES (2, 'after-restore');")
		if err != nil {
			t.Fatalf("failed to insert after recovery: %v", err)
		}

		ctx1, cancel1 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel1()

		err = wait.Until(ctx1, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_bootstrap WHERE id = 2;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-restore"), nil
		})
		if err != nil {
			t.Error("n1→n2 replication broken after bootstrap recovery")
		}

		_, err = testKube.ExecSQL("pgedge-n2-1",
			"INSERT INTO test_bootstrap VALUES (3, 'n2-after-restore');")
		if err != nil {
			t.Fatalf("failed to insert on n2: %v", err)
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel2()

		err = wait.Until(ctx2, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n1-1",
				"SELECT val FROM test_bootstrap WHERE id = 3;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "n2-after-restore"), nil
		})
		if err != nil {
			t.Error("n2→n1 active-active replication broken after bootstrap recovery")
		}
	})
}

// TestIdempotentReconciliation verifies that running init-spock a second time
// on an already-configured cluster produces no side effects — no subscription
// drops/recreates, no slot changes. This catches regressions like the old Python
// script's accidental full-recreation behavior.
func TestIdempotentReconciliation(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-minimal-values.yaml")

	clusterNames := []string{"pgedge-n1", "pgedge-n2"}
	pods := []string{"pgedge-n1-1", "pgedge-n2-1"}

	for _, name := range clusterNames {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// Record subscription identities on each node.
	subsBefore := map[string]string{}
	for _, pod := range pods {
		out, err := testKube.ExecSQL(pod,
			"SELECT sub_id, sub_name FROM spock.subscription ORDER BY sub_name;")
		if err != nil {
			t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
		}
		subsBefore[pod] = strings.TrimSpace(out)
	}

	// Record slot names on each node.
	slotsBefore := map[string]string{}
	for _, pod := range pods {
		out, err := testKube.ExecSQL(pod,
			"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical' ORDER BY slot_name;")
		if err != nil {
			t.Fatalf("failed to query slots on %s: %v", pod, err)
		}
		slotsBefore[pod] = strings.TrimSpace(out)
	}

	// Re-run init-spock.
	upgradeChart(t, "distributed-minimal-values.yaml")

	for _, name := range clusterNames {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy after re-run: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed on re-run: %v\nlogs:\n%s", err, logs)
	}

	// Verify subscriptions were NOT recreated (same sub_id means same subscription).
	t.Run("subscriptions_unchanged", func(t *testing.T) {
		for _, pod := range pods {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT sub_id, sub_name FROM spock.subscription ORDER BY sub_name;")
				if err != nil {
					t.Fatalf("failed to query subscriptions on %s: %v", pod, err)
				}
				after := strings.TrimSpace(out)
				if after != subsBefore[pod] {
					t.Errorf("pod %s: subscriptions changed after idempotent re-run\nbefore: %s\nafter:  %s",
						pod, subsBefore[pod], after)
				}
			})
		}
	})

	// Verify slot names unchanged.
	t.Run("slots_unchanged", func(t *testing.T) {
		for _, pod := range pods {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical' ORDER BY slot_name;")
				if err != nil {
					t.Fatalf("failed to query slots on %s: %v", pod, err)
				}
				after := strings.TrimSpace(out)
				if after != slotsBefore[pod] {
					t.Errorf("pod %s: slots changed after idempotent re-run\nbefore: %s\nafter:  %s",
						pod, slotsBefore[pod], after)
				}
			})
		}
	})

	// Verify replication still works.
	t.Run("replication_still_works", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"CREATE TABLE test_idempotent (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
		_, err = testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_idempotent VALUES (1, 'idempotent-check');")
		if err != nil {
			t.Fatalf("failed to insert: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_idempotent WHERE id = 1;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "idempotent-check"), nil
		})
		if err != nil {
			t.Error("replication broken after idempotent re-run")
		}
	})
}
