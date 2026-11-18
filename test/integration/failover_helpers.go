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

// spockSlotNames returns the sorted names of the logical spk_* slots on pod,
// optionally limited to permanent (non-temporary) ones.
func spockSlotNames(k *kube.Kubectl, pod string, permanentOnly bool) ([]string, error) {
	sql := "SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%'"
	if permanentOnly {
		sql += " AND NOT temporary"
	}
	out, err := k.ExecSQL(pod, sql+" ORDER BY slot_name;")
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

// waitForStandbySlotSync blocks until each standby holds a permanent copy of
// every logical spk_* slot on its primary. Spock's failover_slots worker
// syncs logical slots from primary to standby on an interval (~60s), so a
// primary kill that happens before the first sync promotes a standby with no
// Spock slot and breaks replication.
//
// Since Spock 5.0.14 a new copy starts out as a temporary slot and only
// becomes permanent once the primary's slot has advanced past the standby's
// position. Promotion drops temporary slots, so a copy that merely exists is
// not enough. On an idle primary the slot doesn't advance on its own, so each
// poll calls pg_log_standby_snapshot() on the primary to write a little WAL
// that the subscribers confirm, moving the slot forward.
func waitForStandbySlotSync(t *testing.T, k *kube.Kubectl, syncs []standbySlotSync, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, sync := range syncs {
		for _, pod := range sync.standbys {
			var missing []string
			err := wait.Until(ctx, 5*time.Second, func() (bool, error) {
				if _, err := k.ExecSQL(sync.primary, "SELECT pg_log_standby_snapshot();"); err != nil {
					return false, nil
				}
				want, err := spockSlotNames(k, sync.primary, true)
				if err != nil || len(want) == 0 {
					return false, nil
				}
				have, err := spockSlotNames(k, pod, true)
				if err != nil {
					return false, nil
				}
				missing = missing[:0]
				for _, name := range want {
					if !slices.Contains(have, name) {
						missing = append(missing, name)
					}
				}
				return len(missing) == 0, nil
			})
			if err != nil {
				out, _ := k.ExecSQL(pod,
					"SELECT slot_name, temporary FROM pg_replication_slots WHERE slot_type = 'logical';")
				t.Fatalf("standby %s never received permanent copies of %s's spk_* slots (missing: %v): %v (slots present: %q)",
					pod, sync.primary, missing, err, strings.TrimSpace(out))
			}
			t.Logf("standby %s has permanent copies of %s's spk_* slots", pod, sync.primary)
		}
	}
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
		t.Fatalf("waiting for promotion of %s: %v", cluster, err)
	}
	t.Logf("new primary elected: %s", newPrimary)

	// Make sure the new primary's pod is Ready before tests query it.
	if err := k.WaitForCondition("pod", newPrimary, "Ready", timeout.String()); err != nil {
		t.Fatalf("new primary %s not Ready: %v", newPrimary, err)
	}
}
