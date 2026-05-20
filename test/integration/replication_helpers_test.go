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

// verifyMeshReplication writes one row per pod into the given (int id, text val)
// table, then verifies each row replicates to every other pod. baseID is the
// first integer used; subsequent pods get baseID+1, baseID+2, ... Callers
// must ensure that range doesn't collide with rows already in the table.
// Each (src, dst) pair runs as its own t.Run sub-test for clear failure output.
func verifyMeshReplication(t *testing.T, table string, pods []string, baseID int) {
	t.Helper()

	writes := map[string]int{}
	for i, pod := range pods {
		writes[pod] = baseID + i
	}

	for pod, id := range writes {
		_, err := testKube.ExecSQL(pod,
			fmt.Sprintf("INSERT INTO %s VALUES (%d, 'from-%s');", table, id, pod))
		if err != nil {
			t.Fatalf("failed to insert on %s: %v", pod, err)
		}
	}

	for srcPod, id := range writes {
		for _, dstPod := range pods {
			if dstPod == srcPod {
				continue
			}
			t.Run(fmt.Sprintf("%s_to_%s", srcPod, dstPod), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				expected := fmt.Sprintf("from-%s", srcPod)
				err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
					out, err := testKube.ExecSQL(dstPod,
						fmt.Sprintf("SELECT val FROM %s WHERE id = %d;", table, id))
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
}

// podToNode extracts the node name from a pod name (e.g. "pgedge-n1-1" -> "n1").
func podToNode(pod string) string {
	return strings.TrimSuffix(strings.TrimPrefix(pod, "pgedge-"), "-1")
}
