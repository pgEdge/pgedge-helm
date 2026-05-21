//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

// TestUnplannedFailover verifies that an unplanned CNPG primary failure
// (force-delete the primary pod) results in standby promotion and that
// Spock replication survives in both directions with no data loss.
func TestUnplannedFailover(t *testing.T) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-2node-2instance-values.yaml")

	for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// Seed data on n1 and confirm it reaches n2 BEFORE we kill anything,
	// so the no-data-loss assertion has a known baseline.
	_, err := testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_failover (id int PRIMARY KEY, val text);")
	if err != nil {
		t.Fatalf("create test_failover: %v", err)
	}
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_failover SELECT g, 'pre-failover' FROM generate_series(1, 100) g;")
	if err != nil {
		t.Fatalf("seed test_failover on n1: %v", err)
	}

	waitForReplication(t, []string{"pgedge-n1-1", "pgedge-n2-1"})

	expectedCount, err := testKube.ExecSQL("pgedge-n2-1",
		"SELECT count(*) FROM test_failover;")
	if err != nil {
		t.Fatalf("count test_failover on n2: %v", err)
	}
	expectedCount = strings.TrimSpace(expectedCount)
	if expectedCount != "100" {
		t.Fatalf("expected 100 rows on n2 before failover, got %s", expectedCount)
	}

	// Wait for Spock's failover_slots worker to sync logical slots from
	// each primary to its standby. Without this, killing the primary before
	// the worker's first successful sync leaves the promoted standby with no
	// Spock slot, which breaks replication. The worker runs on an interval
	// (~60s) so the first sync can take up to ~2 minutes from cluster init.
	waitForStandbySlotSync(t, testKube,
		[]string{"pgedge-n1-2", "pgedge-n2-2"}, 3*time.Minute)

	oldPrimary := killPrimary(t, testKube, "pgedge-n1")
	waitForPromotion(t, testKube, "pgedge-n1", oldPrimary, timeout)

	// CNPG keeps the -rw service pointing at the current primary, so
	// queries via the service auto-route to the new primary. But our
	// test helpers exec directly into the pod by name. Re-resolve the
	// primary pod name for subsequent queries.
	newPrimary, err := getPrimaryPod(testKube, "pgedge-n1")
	if err != nil {
		t.Fatalf("locate new primary: %v", err)
	}

	t.Run("no_data_loss", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL(newPrimary,
				"SELECT count(*) FROM test_failover;")
			if err != nil {
				return false, nil
			}
			return strings.TrimSpace(out) == expectedCount, nil
		})
		if err != nil {
			out, _ := testKube.ExecSQL(newPrimary,
				"SELECT count(*) FROM test_failover;")
			t.Errorf("new primary %s count != %s (got %q)", newPrimary, expectedCount, strings.TrimSpace(out))
		}
	})

	t.Run("forward_replication", func(t *testing.T) {
		// Insert on n2, expect to land on the new n1 primary.
		_, err := testKube.ExecSQL("pgedge-n2-1",
			"INSERT INTO test_failover VALUES (1001, 'from-n2-post-failover');")
		if err != nil {
			t.Fatalf("insert on n2: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL(newPrimary,
				"SELECT val FROM test_failover WHERE id = 1001;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "from-n2-post-failover"), nil
		})
		if err != nil {
			t.Error("row from n2 did not reach new n1 primary after failover")
		}
	})

	t.Run("reverse_replication", func(t *testing.T) {
		// Insert on the new n1 primary, expect to land on n2.
		_, err := testKube.ExecSQL(newPrimary,
			"INSERT INTO test_failover VALUES (1002, 'from-new-n1');")
		if err != nil {
			t.Fatalf("insert on new n1 primary %s: %v", newPrimary, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err = wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL("pgedge-n2-1",
				"SELECT val FROM test_failover WHERE id = 1002;")
			if err != nil {
				return false, nil
			}
			return strings.Contains(out, "from-new-n1"), nil
		})
		if err != nil {
			t.Error("row from new n1 primary did not reach n2 after failover")
		}
	})

	t.Run("subscriptions_healthy", func(t *testing.T) {
		for _, tc := range []struct{ pod, label string }{
			{newPrimary, "n1"},
			{"pgedge-n2-1", "n2"},
		} {
			t.Run(tc.label, func(t *testing.T) {
				out, err := testKube.ExecSQL(tc.pod,
					"SELECT (spock.sub_show_status(sub_name)).status FROM spock.subscription;")
				if err != nil {
					t.Fatalf("sub status on %s: %v", tc.pod, err)
				}
				for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
					status := strings.TrimSpace(line)
					if status != "replicating" && status != "initializing" {
						t.Errorf("%s has subscription with bad status %q (full output: %q)",
							tc.pod, status, out)
					}
				}
			})
		}
	})

	t.Run("slots_active", func(t *testing.T) {
		for _, tc := range []struct{ pod, label string }{
			{newPrimary, "n1"},
			{"pgedge-n2-1", "n2"},
		} {
			t.Run(tc.label, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
					out, err := testKube.ExecSQL(tc.pod, `
						SELECT count(*) FROM pg_replication_slots
						WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%' AND active;`)
					if err != nil {
						return false, nil
					}
					return strings.TrimSpace(out) == "1", nil
				})
				if err != nil {
					out, _ := testKube.ExecSQL(tc.pod,
						"SELECT slot_name, active FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%';")
					t.Errorf("%s has inactive spk_* slot(s): %s", tc.pod, strings.TrimSpace(out))
				}
			})
		}
	})

	t.Run("full_mesh_reoplication", func(t *testing.T) {
		verifyMeshReplication(t, "test_failover",
			[]string{newPrimary, "pgedge-n2-1"}, 2001)
	})
}
