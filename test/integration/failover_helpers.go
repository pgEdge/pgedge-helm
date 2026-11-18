//go:build integration

package integration

import (
	"context"
	"fmt"
	"os/exec"
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

// waitForStandbySlotSync blocks until each given standby pod has at least
// one persistent (non-temporary) logical spk_* slot in pg_replication_slots.
//
// The slot copy on a standby is made by Spock's failover_slots worker (Spock
// 5, and Spock 6 on PG16 and older) or by PostgreSQL's native slot sync
// (Spock 6 on PG17+). Either way, the copy starts out temporary and only
// becomes persistent once the slot's consumer on the primary has confirmed
// past the standby's position: Spock 5.0.14+ does this for the worker's
// copies, and native slot sync always has. Promotion drops temporary copies,
// so killing the primary while the copy is still temporary leaves the
// promoted standby with no Spock slot and breaks replication, the same as
// killing it before the first sync.
//
// On an idle cluster nothing moves the consumer forward, so each poll calls
// pg_log_standby_snapshot() on primaryPods to write a running-xacts record
// for the subscribers to decode and confirm.
func waitForStandbySlotSync(t *testing.T, k *kube.Kubectl, primaryPods, standbyPods []string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, pod := range standbyPods {
		pod := pod
		err := wait.Until(ctx, 5*time.Second, func() (bool, error) {
			for _, primary := range primaryPods {
				// Best effort: a failed nudge only delays the copy
				// becoming persistent, and the next poll retries it.
				_, _ = k.ExecSQL(primary, "SELECT pg_log_standby_snapshot();")
			}
			out, err := k.ExecSQL(pod,
				"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%' AND NOT temporary;")
			if err != nil {
				return false, nil
			}
			return strings.TrimSpace(out) != "0", nil
		})
		if err != nil {
			out, _ := k.ExecSQL(pod,
				"SELECT slot_name, temporary FROM pg_replication_slots WHERE slot_type = 'logical';")
			t.Fatalf("standby %s never received a persistent logical spk_* slot: %v (slots present: %q)",
				pod, err, strings.TrimSpace(out))
		}
		t.Logf("standby %s has persistent logical spk_* slot synced", pod)
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
