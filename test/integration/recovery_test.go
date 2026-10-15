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
						"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%';")
					if err != nil {
						return false, nil
					}
					return strings.TrimSpace(out) == "1", nil
				})
				if err != nil {
					// Log the actual slot names for debugging.
					out, _ := testKube.ExecSQL(pod,
						"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%';")
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
					"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%';")
				if err != nil {
					t.Fatalf("failed to query slot names on %s: %v", pod, err)
				}
				if strings.Contains(out, "n3") {
					t.Errorf("pod %s: found slot referencing removed node n3: %s", pod, out)
				}
			})
		}
	})

	// Verify spock.node on both survivors has no entry for n3.
	t.Run("no_n3_spock_node_entries", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT node_name FROM spock.node WHERE node_name = 'n3';")
				if err != nil {
					t.Fatalf("failed to query spock.node on %s: %v", pod, err)
				}
				if strings.Contains(out, "n3") {
					t.Errorf("pod %s: stale spock.node entry for n3 still exists: %s", pod, out)
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

// TestResetSpock verifies that the resetSpock flag drops and recreates all
// Spock state. Simulates stale spock catalog state (foreign node entries,
// wrong node_interface DSN) and verifies resetSpock cleans it up.
func TestResetSpock(t *testing.T) {
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

	// Set up repset state to verify it survives the reset:
	// - A user table added to the default repset
	// - A custom repset with its own table
	t.Run("setup_repsets", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			// Create a table that DDL replication auto-adds to the default repset.
			_, err := testKube.ExecSQL(pod,
				"CREATE TABLE IF NOT EXISTS test_default_repset (id int PRIMARY KEY, val text);")
			if err != nil {
				t.Fatalf("failed to create default repset table on %s: %v", pod, err)
			}

			// Create a custom repset with a table only in that repset.
			// Disable AutoDDL so the table isn't auto-added to default.
			_, err = testKube.ExecSQL(pod, `
				SET spock.enable_ddl_replication = off;
				CREATE TABLE IF NOT EXISTS test_custom_repset (id int PRIMARY KEY, val text);
				SELECT spock.repset_create('custom_repset');
				SELECT spock.repset_add_table('custom_repset', 'test_custom_repset');
			`)
			if err != nil {
				t.Fatalf("failed to create custom repset on %s: %v", pod, err)
			}
		}
	})

	// Simulate a realistic post-restore state: corrupt DSN, drop slots, drop subscription.
	t.Run("simulate_stale_restore", func(t *testing.T) {
		// Corrupt the node_interface DSN.
		_, err := testKube.ExecSQL("pgedge-n1-1", `
			SELECT spock.repair_mode('True');
			UPDATE spock.node_interface SET if_dsn = 'host=CORRUPTED dbname=app user=pgedge port=5432';
		`)
		if err != nil {
			t.Fatalf("failed to corrupt node_interface: %v", err)
		}

		// Drop all subscriptions on n1 (simulates stale/missing subs after restore).
		_, err = testKube.ExecSQL("pgedge-n1-1",
			"SELECT spock.sub_drop(sub_name, true) FROM spock.subscription;")
		if err != nil {
			t.Fatalf("failed to drop subscriptions on n1: %v", err)
		}

		// Drop replication slots on n1 (slots aren't copied during restore).
		_, err = testKube.ExecSQL("pgedge-n1-1", dropSlotsSQL)
		if err != nil {
			t.Fatalf("failed to drop slots on n1: %v", err)
		}

		// Verify the damage.
		out, err := testKube.ExecSQL("pgedge-n1-1",
			"SELECT count(*) FROM spock.subscription;")
		if err != nil {
			t.Fatalf("failed to query subscriptions: %v", err)
		}
		if strings.TrimSpace(out) != "0" {
			t.Fatalf("expected 0 subscriptions after drop, got: %s", out)
		}

		out, err = testKube.ExecSQL("pgedge-n1-1",
			"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical';")
		if err != nil {
			t.Fatalf("failed to query slots: %v", err)
		}
		if strings.TrimSpace(out) != "0" {
			t.Fatalf("expected 0 slots after drop, got: %s", out)
		}
	})

	// Upgrade with resetSpock: true.
	t.Run("reset_via_upgrade", func(t *testing.T) {
		upgradeChart(t, "distributed-reset-spock-values.yaml")

		for _, name := range clusterNames {
			if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
				t.Fatalf("cluster %s not healthy after reset: %v", name, err)
			}
		}
		if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
			logs, _ := testKube.Logs("job/pgedge-init-spock")
			t.Fatalf("init-spock job failed after reset: %v\nlogs:\n%s", err, logs)
		}
	})

	// Verify the corrupted DSN was fixed by the reset.
	t.Run("node_interface_dsn_restored", func(t *testing.T) {
		out, err := testKube.ExecSQL("pgedge-n1-1",
			"SELECT if_dsn FROM spock.node_interface WHERE if_name = 'n1';")
		if err != nil {
			t.Fatalf("failed to query node_interface: %v", err)
		}
		if strings.Contains(out, "CORRUPTED") {
			t.Errorf("node_interface DSN still corrupted after reset: %s", out)
		}
		if !strings.Contains(out, "pgedge-n1-rw") {
			t.Errorf("node_interface DSN doesn't contain expected hostname: %s", out)
		}
	})

	// Verify correct node count and names.
	t.Run("correct_nodes_exist", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				out, err := testKube.ExecSQL(pod,
					"SELECT node_name FROM spock.node ORDER BY node_name;")
				if err != nil {
					t.Fatalf("failed to query spock.node on %s: %v", pod, err)
				}
				got := strings.Fields(strings.TrimSpace(out))
				if len(got) != 2 || got[0] != "n1" || got[1] != "n2" {
					t.Errorf("pod %s: expected exactly [n1 n2], got: %v", pod, got)
				}
			})
		}
	})

	// Verify correct subscription count.
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

	// Verify repset configuration survived the reset.
	t.Run("repsets_preserved", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n2-1"} {
			t.Run(pod, func(t *testing.T) {
				// Custom repset still exists.
				out, err := testKube.ExecSQL(pod,
					"SELECT set_name FROM spock.replication_set WHERE set_name = 'custom_repset';")
				if err != nil {
					t.Fatalf("failed to query repsets on %s: %v", pod, err)
				}
				if !strings.Contains(out, "custom_repset") {
					t.Errorf("pod %s: custom_repset not restored after reset", pod)
				}

				// Custom repset table assignment preserved.
				out, err = testKube.ExecSQL(pod,
					"SELECT set_reloid::regclass::text FROM spock.replication_set_table rst JOIN spock.replication_set rs ON rst.set_id = rs.set_id WHERE rs.set_name = 'custom_repset';")
				if err != nil {
					t.Fatalf("failed to query custom repset tables on %s: %v", pod, err)
				}
				if !strings.Contains(out, "test_custom_repset") {
					t.Errorf("pod %s: test_custom_repset not in custom_repset after reset, got: %s", pod, out)
				}

				// Default repset table assignment preserved.
				out, err = testKube.ExecSQL(pod,
					"SELECT set_reloid::regclass::text FROM spock.replication_set_table rst JOIN spock.replication_set rs ON rst.set_id = rs.set_id WHERE rs.set_name = 'default';")
				if err != nil {
					t.Fatalf("failed to query default repset tables on %s: %v", pod, err)
				}
				if !strings.Contains(out, "test_default_repset") {
					t.Errorf("pod %s: test_default_repset not in default repset after reset, got: %s", pod, out)
				}
			})
		}
	})

	// Verify replication slots restored.
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
					t.Errorf("pod %s: replication slot not restored after reset", pod)
				}
			})
		}
	})

	// Verify replication works after reset.
	t.Run("replication_works_after_reset", func(t *testing.T) {
		_, err := testKube.ExecSQL("pgedge-n1-1",
			"CREATE TABLE IF NOT EXISTS test_reset (id int PRIMARY KEY, val text);")
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
		_, err = testKube.ExecSQL("pgedge-n1-1",
			"INSERT INTO test_reset VALUES (1, 'after-reset');")
		if err != nil {
			t.Fatalf("failed to insert: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_reset WHERE id = 1;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "after-reset"), nil
		})
		if err != nil {
			t.Error("replication broken after resetSpock")
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
