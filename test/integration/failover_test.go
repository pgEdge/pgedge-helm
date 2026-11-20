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

// TestFailoverAndSwitchover promotes node n1's standby three times in a row
// and verifies after each promotion that Spock replication survives in both
// directions with no data loss:
//
//  1. an unplanned failover (force-delete the primary pod);
//  2. a second unplanned failover, once the old primary has rejoined as a
//     standby and its Spock slot copies have become permanent again;
//  3. a planned switchover, the same status patch `kubectl cnpg promote`
//     makes.
//
// Repeated promotions are the risky case with Spock 5.0.14: after each one
// the old primary comes back as a standby whose slot copies have to go
// through the temporary stage again before it can safely be promoted.
func TestFailoverAndSwitchover(t *testing.T) {
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

	// CI runs against a floating image tag, so record which Spock version
	// this run actually exercised. The version is in the subtest name so it
	// shows up in the test output even when the test passes.
	spockVersion, err := testKube.ExecSQL("pgedge-n1-1",
		"SELECT extversion FROM pg_extension WHERE extname = 'spock';")
	if err != nil {
		t.Fatalf("read Spock version: %v", err)
	}
	spockVersion = strings.TrimSpace(spockVersion)
	t.Run("spock_version_"+spockVersion, func(t *testing.T) {
		t.Logf("running against Spock %s", spockVersion)
	})

	// The chart renders spock.failover_slots_database; make sure it reaches
	// Postgres on primaries and standbys alike.
	t.Run("failover_slots_database", func(t *testing.T) {
		for _, pod := range []string{"pgedge-n1-1", "pgedge-n1-2", "pgedge-n2-1", "pgedge-n2-2"} {
			out, err := testKube.ExecSQL(pod,
				"SELECT current_setting('spock.failover_slots_database', true);")
			if err != nil {
				t.Errorf("read spock.failover_slots_database on %s: %v", pod, err)
				continue
			}
			if got := strings.TrimSpace(out); got != "app" {
				t.Errorf("%s: expected spock.failover_slots_database=app, got %q", pod, got)
			}
		}
	})

	// Seed data on n1 and confirm it reaches n2 BEFORE we promote anything,
	// so the no-data-loss assertions have a known baseline.
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_failover (id int PRIMARY KEY, val text);")
	if err != nil {
		t.Fatalf("create test_failover: %v", err)
	}
	_, err = testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_failover SELECT g, 'pre-failover' FROM generate_series(1, 100) g;")
	if err != nil {
		t.Fatalf("seed test_failover on n1: %v", err)
	}

	n1Primary := "pgedge-n1-1"
	for i, step := range []struct {
		name    string
		planned bool
	}{
		{"failover_1", false},
		{"failover_2", false},
		{"switchover", true},
	} {
		n1Standby := otherN1Instance(n1Primary)

		// After a failover the old primary rejoins as a standby. Its slot
		// copies only become permanent once it is streaming again.
		waitForStandby(t, testKube, n1Standby, timeout)
		waitForReplication(t, []string{n1Primary, "pgedge-n2-1"})
		expectedCount := tableCount(t, "pgedge-n2-1", "test_failover")

		// Wait for Spock's failover_slots worker to sync logical slots from
		// each primary to its standby, and for those copies to become
		// permanent. Promoting a standby before then leaves it with no
		// Spock slot, which breaks replication. The worker runs on an
		// interval (~60s) so this can take up to ~2 minutes.
		waitForStandbySlotSync(t, testKube, []standbySlotSync{
			{primary: n1Primary, standbys: []string{n1Standby}},
			{primary: "pgedge-n2-1", standbys: []string{"pgedge-n2-2"}},
		}, 3*time.Minute)

		// Promote only from a healthy cluster, the state in which a user
		// would run `kubectl cnpg promote`.
		if err := wait.ForClusterHealthy(testKube, "pgedge-n1", timeout); err != nil {
			t.Fatalf("%s: cluster pgedge-n1 not healthy before promotion: %v", step.name, err)
		}

		if step.planned {
			switchover(t, testKube, "pgedge-n1", n1Standby)
			waitForPromotion(t, testKube, "pgedge-n1", n1Primary, timeout)
		} else {
			oldPrimary := killPrimary(t, testKube, "pgedge-n1")
			waitForPromotion(t, testKube, "pgedge-n1", oldPrimary, timeout)
		}

		// CNPG keeps the -rw service pointing at the current primary, but
		// our test helpers exec directly into the pod by name, so
		// re-resolve the primary pod name.
		newPrimary, err := getPrimaryPod(testKube, "pgedge-n1")
		if err != nil {
			t.Fatalf("%s: locate new primary: %v", step.name, err)
		}
		if newPrimary != n1Standby {
			t.Fatalf("%s: expected %s to be promoted, got %s", step.name, n1Standby, newPrimary)
		}

		t.Run(step.name, func(t *testing.T) {
			verifyReplicationAfterPromotion(t, newPrimary, expectedCount, (i+1)*1000)
		})
		if t.Failed() {
			t.FailNow()
		}
		n1Primary = newPrimary
	}
}

// otherN1Instance returns n1's other instance in the 2-instance layout.
func otherN1Instance(pod string) string {
	if pod == "pgedge-n1-1" {
		return "pgedge-n1-2"
	}
	return "pgedge-n1-1"
}

// tableCount returns the row count of table on pod as a string.
func tableCount(t *testing.T, pod, table string) string {
	t.Helper()
	out, err := testKube.ExecSQL(pod, fmt.Sprintf("SELECT count(*) FROM %s;", table))
	if err != nil {
		t.Fatalf("count %s on %s: %v", table, pod, err)
	}
	return strings.TrimSpace(out)
}

// verifyReplicationAfterPromotion checks that n1's newly promoted primary
// kept all data and that replication works in both directions. baseID is the
// first row id this round writes; each round uses its own range.
func verifyReplicationAfterPromotion(t *testing.T, newPrimary, expectedCount string, baseID int) {
	t.Run("no_data_loss", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var got string
		err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
			out, err := testKube.ExecSQL(newPrimary, "SELECT count(*) FROM test_failover;")
			if err != nil {
				return false, nil
			}
			got = strings.TrimSpace(out)
			return got == expectedCount, nil
		})
		if err != nil {
			t.Errorf("new primary %s count != %s (got %q)", newPrimary, expectedCount, got)
		}
	})

	t.Run("forward_replication", func(t *testing.T) {
		// Insert on n2, expect to land on the new n1 primary.
		id := baseID + 1
		_, err := testKube.ExecSQL("pgedge-n2-1",
			fmt.Sprintf("INSERT INTO test_failover VALUES (%d, 'from-n2-post-promotion');", id))
		if err != nil {
			t.Fatalf("insert on n2: %v", err)
		}
		if !waitForRow(newPrimary, id, "from-n2-post-promotion") {
			t.Errorf("row %d from n2 did not reach new n1 primary %s", id, newPrimary)
		}
	})

	t.Run("reverse_replication", func(t *testing.T) {
		// Insert on the new n1 primary, expect to land on n2.
		id := baseID + 2
		_, err := testKube.ExecSQL(newPrimary,
			fmt.Sprintf("INSERT INTO test_failover VALUES (%d, 'from-new-n1');", id))
		if err != nil {
			t.Fatalf("insert on new n1 primary %s: %v", newPrimary, err)
		}
		if !waitForRow("pgedge-n2-1", id, "from-new-n1") {
			t.Errorf("row %d from new n1 primary %s did not reach n2", id, newPrimary)
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
						WHERE plugin = 'spock_output' AND NOT temporary AND active;`)
					if err != nil {
						return false, nil
					}
					return strings.TrimSpace(out) == "1", nil
				})
				if err != nil {
					out, _ := testKube.ExecSQL(tc.pod,
						"SELECT slot_name, temporary, active FROM pg_replication_slots WHERE plugin = 'spock_output';")
					t.Errorf("%s has no permanent, active Spock slot: %s", tc.pod, strings.TrimSpace(out))
				}
			})
		}
	})

	t.Run("full_mesh_replication", func(t *testing.T) {
		verifyMeshReplication(t, "test_failover",
			[]string{newPrimary, "pgedge-n2-1"}, baseID+10)
	})
}

// waitForRow reports whether the row with the given id and a val containing
// want shows up on pod within 60 seconds.
func waitForRow(pod string, id int, want string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
		out, err := testKube.ExecSQL(pod,
			fmt.Sprintf("SELECT val FROM test_failover WHERE id = %d;", id))
		if err != nil {
			return false, nil
		}
		return strings.Contains(out, want), nil
	})
	return err == nil
}
