//go:build integration

package integration

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/kube"
	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

const (
	haLoadTable     = "test_ha_load"
	haLoadBatchSize = 20
)

// loadWriter inserts batches of rows into haLoadTable on the current primary
// of a CNPG cluster until stopped, and records which batches committed.
//
// A batch whose INSERT returned without error is committed. A batch that
// failed (e.g. while the primary was being switched) may or may not have
// committed, so it is recorded as uncertain and not checked.
type loadWriter struct {
	cluster string
	nextID  int

	mu        sync.Mutex
	committed []int // first id of each committed batch
	uncertain int

	stop chan struct{}
	done chan struct{}
}

func startLoadWriter(t *testing.T, k *kube.Kubectl, cluster string, firstID int) *loadWriter {
	t.Helper()
	w := &loadWriter{
		cluster: cluster,
		nextID:  firstID,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go func() {
		defer close(w.done)
		for {
			select {
			case <-w.stop:
				return
			default:
			}
			pod, err := getPrimaryPod(k, w.cluster)
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			first := w.nextID
			w.nextID += haLoadBatchSize
			_, err = k.ExecSQL(pod, fmt.Sprintf(
				"INSERT INTO %s SELECT g, '%s' FROM generate_series(%d, %d) g;",
				haLoadTable, w.cluster, first, first+haLoadBatchSize-1))
			w.mu.Lock()
			if err == nil {
				w.committed = append(w.committed, first)
			} else {
				w.uncertain++
			}
			w.mu.Unlock()
		}
	}()
	return w
}

// stopAndWait stops the writer and returns the ids of every committed row.
func (w *loadWriter) stopAndWait(t *testing.T) []int {
	t.Helper()
	close(w.stop)
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	var ids []int
	for _, first := range w.committed {
		for i := 0; i < haLoadBatchSize; i++ {
			ids = append(ids, first+i)
		}
	}
	t.Logf("writer on %s: %d committed batches (%d rows), %d uncertain batches",
		w.cluster, len(w.committed), len(ids), w.uncertain)
	return ids
}

// assertRowsPresent checks that every id in want exists on pod.
func assertRowsPresent(t *testing.T, pod string, want []int, label string) {
	t.Helper()
	if len(want) == 0 {
		t.Errorf("%s: writer committed no rows, so the test proved nothing", label)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var missing []int
	err := wait.Until(ctx, 3*time.Second, func() (bool, error) {
		out, err := testKube.ExecSQL(pod, fmt.Sprintf(
			"SELECT id FROM %s WHERE id BETWEEN %d AND %d ORDER BY id;",
			haLoadTable, want[0], want[len(want)-1]))
		if err != nil {
			return false, nil
		}
		have := map[int]bool{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if id, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
				have[id] = true
			}
		}
		missing = missing[:0]
		for _, id := range want {
			if !have[id] {
				missing = append(missing, id)
			}
		}
		return len(missing) == 0, nil
	})
	if err != nil {
		sort.Ints(missing)
		shown := missing
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Errorf("%s: %d of %d committed rows missing on %s (first: %v)",
			label, len(missing), len(want), pod, shown)
	}
}

// standbyPod returns a pod of the cluster that isn't primary.
func standbyPod(t *testing.T, cluster, primary string) string {
	t.Helper()
	for _, suffix := range []string{"-1", "-2", "-3"} {
		if pod := cluster + suffix; pod != primary {
			return pod
		}
	}
	t.Fatalf("no standby found for %s", cluster)
	return ""
}

// runHAUnderLoad installs two nodes with two instances each, keeps writing
// to both nodes, changes n1's primary with promote, and checks that no
// committed row is lost and that replication works both ways afterwards.
//
// Each node has a synchronous standby, so every acknowledged commit is on
// the standby before the primary goes away. With asynchronous replication a
// force-deleted primary can lose acknowledged commits by design, and the
// zero-loss check would fail without any fault in Spock or the chart.
func runHAUnderLoad(t *testing.T, promote func(t *testing.T, oldPrimary string)) {
	t.Cleanup(func() { uninstallChart(t) })
	installChart(t, "distributed-2node-2instance-sync-values.yaml")

	for _, name := range []string{"pgedge-n1", "pgedge-n2"} {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}

	// DDL replication creates the table on n2 as well.
	if _, err := testKube.ExecSQL("pgedge-n1-1", fmt.Sprintf(
		"CREATE TABLE %s (id int PRIMARY KEY, val text);", haLoadTable)); err != nil {
		t.Fatalf("create %s: %v", haLoadTable, err)
	}
	waitForReplication(t, []string{"pgedge-n1-1", "pgedge-n2-1"})

	// On PostgreSQL 17+ every Spock slot must carry the failover flag for
	// native slot sync; the job sets it on all of them.
	assertSpockSlotsHaveFailoverFlag(t, "pgedge-n1-1")
	assertSpockSlotsHaveFailoverFlag(t, "pgedge-n2-1")

	waitForStandbySlotSync(t, testKube, []standbySlotSync{
		{primary: "pgedge-n1-1", standbys: []string{"pgedge-n1-2"}},
		{primary: "pgedge-n2-1", standbys: []string{"pgedge-n2-2"}},
	}, 3*time.Minute)

	n1Writer := startLoadWriter(t, testKube, "pgedge-n1", 1_000_000)
	n2Writer := startLoadWriter(t, testKube, "pgedge-n2", 2_000_000)
	writersStopped := false
	defer func() {
		if !writersStopped {
			n1Writer.stopAndWait(t)
			n2Writer.stopAndWait(t)
		}
	}()

	// Let some load build up before changing the primary.
	time.Sleep(10 * time.Second)

	oldPrimary, err := getPrimaryPod(testKube, "pgedge-n1")
	if err != nil {
		t.Fatalf("locate n1 primary: %v", err)
	}
	promote(t, oldPrimary)
	waitForPromotion(t, testKube, "pgedge-n1", oldPrimary, timeout)

	// Keep writing on the new primary for a while.
	time.Sleep(20 * time.Second)
	n1IDs := n1Writer.stopAndWait(t)
	n2IDs := n2Writer.stopAndWait(t)
	writersStopped = true

	newPrimary, err := getPrimaryPod(testKube, "pgedge-n1")
	if err != nil {
		t.Fatalf("locate new n1 primary: %v", err)
	}
	pods := []string{newPrimary, "pgedge-n2-1"}
	waitForReplication(t, pods)

	t.Run("no_committed_row_lost", func(t *testing.T) {
		for _, pod := range pods {
			assertRowsPresent(t, pod, n1IDs, "rows written on n1")
			assertRowsPresent(t, pod, n2IDs, "rows written on n2")
		}
	})

	t.Run("subscriptions_replicating", func(t *testing.T) {
		for _, pod := range pods {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			var out string
			err := wait.Until(ctx, 3*time.Second, func() (bool, error) {
				var err error
				out, err = testKube.ExecSQL(pod,
					"SELECT string_agg(status, ',') FROM spock.sub_show_status();")
				return err == nil && strings.TrimSpace(out) == "replicating", nil
			})
			cancel()
			if err != nil {
				t.Errorf("%s: subscription status %q, want replicating", pod, strings.TrimSpace(out))
			}
		}
	})

	t.Run("full_mesh_replication", func(t *testing.T) {
		if _, err := testKube.ExecSQL(newPrimary, "CREATE TABLE test_ha_mesh (id int PRIMARY KEY, val text);"); err != nil {
			t.Fatalf("create test_ha_mesh: %v", err)
		}
		waitForReplication(t, pods)
		verifyMeshReplication(t, "test_ha_mesh", pods, 1)
	})
}

// TestSwitchoverUnderLoad checks that a planned CNPG switchover of one node,
// with writes running on both nodes, keeps every committed row and leaves
// replication working in both directions.
func TestSwitchoverUnderLoad(t *testing.T) {
	runHAUnderLoad(t, func(t *testing.T, oldPrimary string) {
		switchover(t, testKube, "pgedge-n1", standbyPod(t, "pgedge-n1", oldPrimary))
	})
}

// TestFailoverUnderLoad checks the same for an unplanned failover (the
// primary pod is force-deleted).
func TestFailoverUnderLoad(t *testing.T) {
	runHAUnderLoad(t, func(t *testing.T, oldPrimary string) {
		killPrimary(t, testKube, "pgedge-n1")
	})
}

// assertSpockSlotsHaveFailoverFlag checks, on PostgreSQL 17 and later with
// Spock 6.0 or later, that every Spock slot on pod has the failover flag
// that native slot sync needs. Other versions don't use it.
func assertSpockSlotsHaveFailoverFlag(t *testing.T, pod string) {
	t.Helper()
	out, err := testKube.ExecSQL(pod, `
		SELECT current_setting('server_version_num')::int >= 170000
		   AND to_regprocedure('spock.slot_enable_failover()') IS NOT NULL;`)
	if err != nil {
		t.Fatalf("check versions on %s: %v", pod, err)
	}
	if strings.TrimSpace(out) != "t" {
		return
	}
	out, err = testKube.ExecSQL(pod, `
		SELECT coalesce(string_agg(slot_name, ','), '')
		FROM pg_replication_slots
		WHERE plugin = 'spock_output' AND NOT temporary AND NOT failover;`)
	if err != nil {
		t.Fatalf("list slots on %s: %v", pod, err)
	}
	if s := strings.TrimSpace(out); s != "" {
		t.Errorf("%s: Spock slots without the failover flag: %s", pod, s)
	}
}
