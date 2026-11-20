//go:build integration

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/kube"
	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

// getPrimaryPod returns the pod name of the current primary for the given
// CNPG cluster, identified via the cnpg.io/instanceRole=primary label.
func getPrimaryPod(k *kube.Kubectl, cluster string) (string, error) {
	args := []string{}
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	if k.Namespace != "" {
		args = append(args, "-n", k.Namespace)
	}
	args = append(args,
		"get", "pod",
		"-l", fmt.Sprintf("cnpg.io/cluster=%s,cnpg.io/instanceRole=primary", cluster),
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("get primary pod for %s: %w (%s)", cluster, err, string(out))
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no primary pod found for cluster %s", cluster)
	}
	return name, nil
}

// killPrimary force-deletes the current primary pod of the given cluster.
// CNPG will promote a standby in response. Returns the name of the killed pod.
func killPrimary(t *testing.T, k *kube.Kubectl, cluster string) string {
	t.Helper()
	primary, err := getPrimaryPod(k, cluster)
	if err != nil {
		t.Fatalf("locate primary for %s: %v", cluster, err)
	}

	args := []string{}
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	if k.Namespace != "" {
		args = append(args, "-n", k.Namespace)
	}
	args = append(args, "delete", "pod", primary,
		"--grace-period=0", "--force", "--wait=false")

	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("kill primary %s: %v (%s)", primary, err, string(out))
	}
	t.Logf("force-deleted primary pod %s", primary)
	return primary
}

// standbySlotSync names a CNPG cluster's primary pod and the standby pods
// that must hold copies of its Spock slots before a failover.
type standbySlotSync struct {
	primary  string
	standbys []string
}

// spockSlotNames returns the sorted names of the permanent, usable Spock
// (spock_output) slots on pod. A slot whose WAL was removed
// (wal_status = 'lost') or that was invalidated by a recovery conflict
// (conflicting, Postgres 16+) can't serve a subscriber after a promotion, so
// it doesn't count.
func spockSlotNames(k *kube.Kubectl, pod string) ([]string, error) {
	out, err := k.ExecSQL(pod, `
		SELECT slot_name FROM pg_replication_slots
		WHERE plugin = 'spock_output' AND NOT temporary
		  AND wal_status IS DISTINCT FROM 'lost'
		  AND conflicting IS NOT TRUE
		ORDER BY slot_name;`)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// waitForStandbySlotSync blocks until each standby holds a permanent, usable
// copy of every Spock slot on its primary. Spock's failover_slots worker
// syncs logical slots from primary to standby on an interval (~60s), so a
// primary kill that happens before the first sync promotes a standby with no
// Spock slot and breaks replication.
//
// Since Spock 5.0.14 a new copy starts out as a temporary slot, and promotion
// drops temporary slots, so a copy that merely exists is not enough. All
// pairs are checked in every round so they share the timeout fairly.
//
// Each round also calls pg_log_standby_snapshot() on every primary. This
// works around a known issue in Spock 5.0.14, where a copy can stay
// temporary on an idle primary even after it has caught up: the call writes
// a little WAL that the subscribers confirm, which moves the primary's slot
// forward and lets the copy become permanent.
//
// TODO: drop the pg_log_standby_snapshot() call once the chart's default
// image ships a Spock release that fixes the idle-primary issue.
func waitForStandbySlotSync(t *testing.T, k *kube.Kubectl, syncs []standbySlotSync, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// pending maps each standby that wasn't ready in the last round to the
	// reason why. It is rebuilt from scratch every round, so a standby that
	// passed earlier but whose copy has since been lost or invalidated is
	// caught: the wait only succeeds when every standby passes in the same
	// round.
	pending := map[string]string{}
	for _, pair := range syncs {
		for _, pod := range pair.standbys {
			pending[pod] = "not checked yet"
		}
	}
	logged := map[string]bool{}

	err := wait.Until(ctx, 5*time.Second, func() (bool, error) {
		round := map[string]string{}
		for _, pair := range syncs {
			if _, err := k.ExecSQL(pair.primary, "SELECT pg_log_standby_snapshot();"); err != nil {
				for _, pod := range pair.standbys {
					round[pod] = fmt.Sprintf("pg_log_standby_snapshot() on %s failed: %v", pair.primary, err)
				}
				continue
			}
			want, err := spockSlotNames(k, pair.primary)
			if err != nil || len(want) == 0 {
				for _, pod := range pair.standbys {
					if err != nil {
						round[pod] = fmt.Sprintf("listing slots on %s failed: %v", pair.primary, err)
					} else {
						round[pod] = fmt.Sprintf("%s has no Spock slots yet", pair.primary)
					}
				}
				continue
			}
			for _, pod := range pair.standbys {
				have, err := spockSlotNames(k, pod)
				if err != nil {
					round[pod] = fmt.Sprintf("listing slots on %s failed: %v", pod, err)
					continue
				}
				var missing []string
				for _, name := range want {
					if !slices.Contains(have, name) {
						missing = append(missing, name)
					}
				}
				if len(missing) > 0 {
					round[pod] = fmt.Sprintf("no permanent, usable copy of %v from %s", missing, pair.primary)
					continue
				}
				if !logged[pod] {
					logged[pod] = true
					t.Logf("standby %s has permanent copies of %s's Spock slots", pod, pair.primary)
				}
			}
		}
		pending = round
		return len(pending) == 0, nil
	})
	if err != nil {
		for pod, reason := range pending {
			out, _ := k.ExecSQL(pod,
				"SELECT slot_name, temporary, wal_status, conflicting FROM pg_replication_slots WHERE plugin = 'spock_output';")
			t.Errorf("standby %s not ready: %s (slots present: %q)", pod, reason, strings.TrimSpace(out))
		}
		t.Fatalf("standbys never received permanent copies of their primary's Spock slots: %v", err)
	}
}

// switchover asks CNPG to promote target in the given cluster. It makes the
// same status patch as `kubectl cnpg promote <cluster> <target>`, so the test
// doesn't need the cnpg kubectl plugin.
func switchover(t *testing.T, k *kube.Kubectl, cluster, target string) {
	t.Helper()
	patch := fmt.Sprintf(
		`{"status":{"targetPrimary":%q,"targetPrimaryTimestamp":%q,"phase":"Switchover in progress","phaseReason":"Switching over to %s"}}`,
		target, time.Now().Format("2006-01-02T15:04:05.000000Z07:00"), target)

	args := []string{}
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	if k.Namespace != "" {
		args = append(args, "-n", k.Namespace)
	}
	args = append(args, "patch", "clusters.postgresql.cnpg.io", cluster,
		"--subresource=status", "--type=merge", "-p", patch)

	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("switchover %s to %s: %v (%s)", cluster, target, err, string(out))
	}

	// The patch isn't conditional, so make sure the request stuck.
	got, err := clusterStatusField(k, cluster, "{.status.targetPrimary}")
	if err != nil || got != target {
		t.Fatalf("switchover %s to %s: status.targetPrimary is %q after patch (err: %v)", cluster, target, got, err)
	}
	t.Logf("requested switchover of %s to %s", cluster, target)
}

// clusterStatusField reads a jsonpath expression from a CNPG Cluster.
func clusterStatusField(k *kube.Kubectl, cluster, jsonpath string) (string, error) {
	args := []string{}
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	if k.Namespace != "" {
		args = append(args, "-n", k.Namespace)
	}
	args = append(args, "get", "clusters.postgresql.cnpg.io", cluster, "-o", "jsonpath="+jsonpath)
	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w (%s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// waitForStandby blocks until pod is a standby that is streaming from its
// primary. After a failover CNPG recreates the old primary as a standby, and
// its Spock slot copies can only become permanent once it streams again.
func waitForStandby(t *testing.T, k *kube.Kubectl, pod string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var last string
	err := wait.Until(ctx, 5*time.Second, func() (bool, error) {
		out, err := k.ExecSQL(pod,
			"SELECT pg_is_in_recovery() AND EXISTS (SELECT 1 FROM pg_stat_wal_receiver WHERE status = 'streaming');")
		if err != nil {
			last = err.Error()
			return false, nil
		}
		last = strings.TrimSpace(out)
		return last == "t", nil
	})
	if err != nil {
		t.Fatalf("%s did not become a streaming standby: %v (last check: %s)", pod, err, last)
	}
	t.Logf("%s is a streaming standby", pod)
}

// waitForPromotion blocks until CNPG promotes a new primary for the cluster
// (different from oldPrimary) and that new primary is Ready.
func waitForPromotion(t *testing.T, k *kube.Kubectl, cluster, oldPrimary string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var newPrimary string
	err := wait.Until(ctx, 2*time.Second, func() (bool, error) {
		p, err := getPrimaryPod(k, cluster)
		if err != nil {
			// Transient — primary may not yet be elected.
			return false, nil
		}
		if p == oldPrimary {
			return false, nil
		}
		newPrimary = p
		return true, nil
	})
	if err != nil {
		status, _ := clusterStatusField(k, cluster,
			"currentPrimary={.status.currentPrimary} targetPrimary={.status.targetPrimary} phase={.status.phase} reason={.status.phaseReason}")
		t.Fatalf("waiting for promotion of %s: %v (%s)", cluster, err, status)
	}
	t.Logf("new primary elected: %s", newPrimary)

	// Make sure the new primary's pod is Ready before tests query it.
	if err := k.WaitForCondition("pod", newPrimary, "Ready", timeout.String()); err != nil {
		t.Fatalf("new primary %s not Ready: %v", newPrimary, err)
	}
}
