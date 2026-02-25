package wait

import (
	"context"
	"fmt"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/kube"
)

// Until polls fn at the given interval until it returns true or the context expires.
func Until(ctx context.Context, interval time.Duration, fn func() (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// ForClusterHealthy waits for a CNPG Cluster to reach "Ready" condition.
func ForClusterHealthy(k *kube.Kubectl, clusterName string, timeout time.Duration) error {
	return k.WaitForCondition("clusters.postgresql.cnpg.io", clusterName, "Ready", timeout.String())
}

// ForJobComplete waits for a Job to reach "Complete" condition.
func ForJobComplete(k *kube.Kubectl, jobName string, timeout time.Duration) error {
	return k.WaitForCondition("job", jobName, "Complete", timeout.String())
}

// ForCertReady waits for a cert-manager Certificate to be Ready.
func ForCertReady(k *kube.Kubectl, certName string, timeout time.Duration) error {
	return k.WaitForCondition("certificate.cert-manager.io", certName, "Ready", timeout.String())
}
