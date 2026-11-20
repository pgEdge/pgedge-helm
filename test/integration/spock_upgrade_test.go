//go:build integration

package integration

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/helm"
	"github.com/pgEdge/pgedge-helm/test/pkg/wait"
)

// TestSpockMajorUpgrade installs the chart on UPGRADE_FROM_POSTGRES_IMAGE
// (e.g. a Spock 5.0.14 image), upgrades the release to POSTGRES_IMAGE (e.g.
// a Spock 6.0 image of the same PostgreSQL major), and checks that:
//
//   - the Spock extension on every node is updated to the new binaries,
//   - existing data is kept and replication works both ways,
//   - on PostgreSQL 17+ every Spock slot ends up with the failover flag,
//   - each standby has usable copies of its primary's Spock slots.
//
// It is skipped unless both images are set.
func TestSpockMajorUpgrade(t *testing.T) {
	fromImg := os.Getenv("UPGRADE_FROM_POSTGRES_IMAGE")
	if fromImg == "" || postgresImg == "" {
		t.Skip("set UPGRADE_FROM_POSTGRES_IMAGE and POSTGRES_IMAGE to run the Spock upgrade test")
	}
	t.Logf("upgrading from %s to %s", fromImg, postgresImg)

	t.Cleanup(func() { uninstallChart(t) })

	opts := helm.InstallOpts{
		ChartRef:    chartRef,
		Version:     chartVersion,
		ValuesFiles: []string{testdataPath("distributed-2node-2instance-values.yaml")},
		SetValues:   []string{fmt.Sprintf("pgEdge.clusterSpec.imageName=%s", fromImg)},
		Wait:        true,
		Timeout:     timeout.String(),
	}
	if initSpockImg != "" {
		opts.SetValues = append(opts.SetValues, fmt.Sprintf("pgEdge.initSpockImageName=%s", initSpockImg))
	}
	if err := testHelm.Install(helmRelease, opts); err != nil {
		t.Fatalf("helm install failed: %v", err)
	}
	waitForClustersAndJob(t, "pgedge-n1", "pgedge-n2")

	if _, err := testKube.ExecSQL("pgedge-n1-1",
		"CREATE TABLE test_upgrade (id int PRIMARY KEY, val text);"); err != nil {
		t.Fatalf("create test_upgrade: %v", err)
	}
	if _, err := testKube.ExecSQL("pgedge-n1-1",
		"INSERT INTO test_upgrade SELECT g, 'before-upgrade' FROM generate_series(1, 500) g;"); err != nil {
		t.Fatalf("seed test_upgrade: %v", err)
	}
	waitForReplication(t, []string{"pgedge-n1-1", "pgedge-n2-1"})
	before := extensionVersion(t, "pgedge-n1-1")

	// Upgrade the release to POSTGRES_IMAGE. CNPG rolls each node's
	// instances onto the new image, and init-spock runs again afterwards.
	upgradeChart(t, "distributed-2node-2instance-values.yaml")
	waitForClustersAndJob(t, "pgedge-n1", "pgedge-n2")

	primaries := map[string]string{}
	for _, cluster := range []string{"pgedge-n1", "pgedge-n2"} {
		pod, err := getPrimaryPod(testKube, cluster)
		if err != nil {
			t.Fatalf("locate primary of %s: %v", cluster, err)
		}
		primaries[cluster] = pod
	}
	pods := []string{primaries["pgedge-n1"], primaries["pgedge-n2"]}

	t.Run("extension_updated", func(t *testing.T) {
		for _, pod := range pods {
			out, err := testKube.ExecSQL(pod, `
				SELECT e.extversion || '|' || a.default_version
				FROM pg_extension e JOIN pg_available_extensions a ON a.name = e.extname
				WHERE e.extname = 'spock';`)
			if err != nil {
				t.Fatalf("read spock version on %s: %v", pod, err)
			}
			parts := strings.Split(strings.TrimSpace(out), "|")
			if len(parts) != 2 || parts[0] != parts[1] {
				t.Errorf("%s: spock extension %q does not match the binaries %q", pod, parts[0], parts[len(parts)-1])
			}
			// Make sure the images really were a major upgrade, so the
			// test can't pass with two images of the same Spock major.
			fromMajor, toMajor := spockMajorOf(before), spockMajorOf(parts[0])
			if fromMajor < 0 || toMajor <= fromMajor {
				t.Errorf("%s: expected a Spock major upgrade, got %s -> %s", pod, before, parts[0])
			}
			t.Logf("%s: spock %s -> %s", pod, before, parts[0])
		}
	})

	t.Run("data_kept", func(t *testing.T) {
		for _, pod := range pods {
			out, err := testKube.ExecSQL(pod, "SELECT count(*) FROM test_upgrade;")
			if err != nil {
				t.Fatalf("count on %s: %v", pod, err)
			}
			if strings.TrimSpace(out) != "500" {
				t.Errorf("%s: expected 500 rows, got %s", pod, strings.TrimSpace(out))
			}
		}
	})

	t.Run("replication", func(t *testing.T) {
		waitForReplication(t, pods)
		verifyMeshReplication(t, "test_upgrade", pods, 1001)
	})

	t.Run("slots_have_failover_flag", func(t *testing.T) {
		for _, pod := range pods {
			assertSpockSlotsHaveFailoverFlag(t, pod)
		}
	})

	t.Run("standbys_have_slot_copies", func(t *testing.T) {
		var syncs []standbySlotSync
		var standbys []string
		for cluster, primary := range primaries {
			standby := standbyPod(t, cluster, primary)
			standbys = append(standbys, standby)
			syncs = append(syncs, standbySlotSync{primary: primary, standbys: []string{standby}})
		}

		// Where native slot sync takes over (PostgreSQL 18 + Spock 6.0), the
		// copies Spock 5's worker made are not synced copies and native slot
		// sync won't replace them. Drop them as the upgrade guide says.
		native := nativeSlotSyncOn(t, primaries["pgedge-n1"])
		if native {
			for _, standby := range standbys {
				if _, err := testKube.ExecSQL(standby, `
					SELECT pg_drop_replication_slot(slot_name)
					FROM pg_replication_slots
					WHERE plugin = 'spock_output' AND NOT synced AND NOT active;`); err != nil {
					t.Fatalf("drop Spock 5 slot copies on %s: %v", standby, err)
				}
			}
		}

		waitForStandbySlotSync(t, testKube, syncs, 5*time.Minute)

		if native {
			for _, standby := range standbys {
				out, err := testKube.ExecSQL(standby, `
					SELECT coalesce(string_agg(slot_name, ','), '')
					FROM pg_replication_slots
					WHERE plugin = 'spock_output' AND NOT synced;`)
				if err != nil {
					t.Fatalf("list slots on %s: %v", standby, err)
				}
				if s := strings.TrimSpace(out); s != "" {
					t.Errorf("%s: Spock slot copies not made by native slot sync: %s", standby, s)
				}
			}
		}
	})
}

// nativeSlotSyncOn reports whether pod has sync_replication_slots on, i.e.
// PostgreSQL's native slot sync copies the Spock slots to its standbys.
func nativeSlotSyncOn(t *testing.T, pod string) bool {
	t.Helper()
	out, err := testKube.ExecSQL(pod, "SELECT current_setting('sync_replication_slots', true);")
	if err != nil {
		t.Fatalf("read sync_replication_slots on %s: %v", pod, err)
	}
	return strings.TrimSpace(out) == "on"
}

func waitForClustersAndJob(t *testing.T, clusters ...string) {
	t.Helper()
	for _, name := range clusters {
		if err := wait.ForClusterHealthy(testKube, name, timeout); err != nil {
			t.Fatalf("cluster %s not healthy: %v", name, err)
		}
	}
	if err := wait.ForJobComplete(testKube, "pgedge-init-spock", timeout); err != nil {
		logs, _ := testKube.Logs("job/pgedge-init-spock")
		t.Fatalf("init-spock job failed: %v\nlogs:\n%s", err, logs)
	}
}

func extensionVersion(t *testing.T, pod string) string {
	t.Helper()
	out, err := testKube.ExecSQL(pod, "SELECT extversion FROM pg_extension WHERE extname = 'spock';")
	if err != nil {
		t.Fatalf("read spock version on %s: %v", pod, err)
	}
	return strings.TrimSpace(out)
}

// spockMajorOf returns the major version of a Spock version string such as
// "5.0.14" or "6.0.0-beta.2", or -1 if it can't be parsed.
func spockMajorOf(version string) int {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return -1
	}
	return n
}
