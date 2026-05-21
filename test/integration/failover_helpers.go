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
// one logical spk_* slot in pg_replication_slots. Spock's failover_slots
// worker syncs logical slots from primary to standby on an interval (~60s).
// Without this wait, a primary kill that happens before the first sync will
// promote a standby that has no Spock slot, breaking replication.
func waitForStandbySlotSync(t *testing.T, k *kube.Kubectl, standbyPods []string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, pod := range standbyPods {
		pod := pod
		err := wait.Until(ctx, 5*time.Second, func() (bool, error) {
			out, err := k.ExecSQL(pod,
				"SELECT count(*) FROM pg_replication_slots WHERE slot_type = 'logical' AND slot_name LIKE 'spk_%';")
			if err != nil {
				return false, nil
			}
			return strings.TrimSpace(out) != "0", nil
		})
		if err != nil {
			out, _ := k.ExecSQL(pod,
				"SELECT slot_name FROM pg_replication_slots WHERE slot_type = 'logical';")
			t.Fatalf("standby %s never received a logical spk_* slot: %v (slots present: %q)",
				pod, err, strings.TrimSpace(out))
		}
		t.Logf("standby %s has logical spk_* slot synced", pod)
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
