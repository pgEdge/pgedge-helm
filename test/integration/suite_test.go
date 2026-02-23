//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pgEdge/pgedge-helm/test/pkg/helm"
	"github.com/pgEdge/pgedge-helm/test/pkg/kube"
)

var (
	kubeContext   string
	helmRelease  string
	namespace    string
	chartRef     string
	chartVersion string
	helmRepo     string
	initSpockImg string
	timeout      time.Duration
	testHelm     *helm.Helm
	testKube     *kube.Kubectl
)

func TestMain(m *testing.M) {
	kubeContext = envOrDefault("KUBECONTEXT", "kind-pgedge-test")
	helmRelease = envOrDefault("HELM_RELEASE", "pgedge")
	namespace = envOrDefault("NAMESPACE", "default")
	chartRef = envOrDefault("CHART_REF", defaultChartRef())
	chartVersion = os.Getenv("CHART_VERSION")
	helmRepo = os.Getenv("HELM_REPO")
	initSpockImg = os.Getenv("INIT_SPOCK_IMAGE")

	timeoutStr := envOrDefault("TIMEOUT", "300s")
	var err error
	timeout, err = time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid TIMEOUT %q: %v\n", timeoutStr, err)
		os.Exit(1)
	}

	testHelm = &helm.Helm{KubeContext: kubeContext, Namespace: namespace}
	testKube = &kube.Kubectl{Context: kubeContext, Namespace: namespace}

	if helmRepo != "" {
		repoName := strings.Split(chartRef, "/")[0]
		if err := testHelm.RepoAdd(repoName, helmRepo); err != nil {
			fmt.Fprintf(os.Stderr, "helm repo add failed: %v\n", err)
			os.Exit(1)
		}
	}

	if err := checkPrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "prerequisite check failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func checkPrerequisites() error {
	cnpgKube := &kube.Kubectl{Context: kubeContext, Namespace: "cnpg-system"}
	_, err := cnpgKube.Get("deployment", "cnpg-controller-manager")
	if err != nil {
		return fmt.Errorf("CNPG operator not found in cnpg-system namespace: %w", err)
	}

	cmKube := &kube.Kubectl{Context: kubeContext, Namespace: "cert-manager"}
	_, err = cmKube.Get("deployment", "cert-manager")
	if err != nil {
		return fmt.Errorf("cert-manager not found in cert-manager namespace: %w", err)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultChartRef() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

func installChart(t *testing.T, valuesFile string) {
	t.Helper()
	opts := helm.InstallOpts{
		ChartRef:    chartRef,
		Version:     chartVersion,
		ValuesFiles: []string{testdataPath(valuesFile)},
		Wait:        true,
		Timeout:     timeout.String(),
	}
	if initSpockImg != "" {
		opts.SetValues = []string{fmt.Sprintf("pgEdge.initSpockImageName=%s", initSpockImg)}
	}
	if err := testHelm.Install(helmRelease, opts); err != nil {
		t.Fatalf("helm install failed: %v", err)
	}
}

func upgradeChart(t *testing.T, valuesFile string) {
	t.Helper()
	if err := tryUpgradeChart(valuesFile); err != nil {
		t.Fatalf("helm upgrade failed: %v", err)
	}
}

func tryUpgradeChart(valuesFile string) error {
	opts := helm.UpgradeOpts{
		ChartRef:    chartRef,
		Version:     chartVersion,
		ValuesFiles: []string{testdataPath(valuesFile)},
		Wait:        true,
		Timeout:     timeout.String(),
	}
	if initSpockImg != "" {
		opts.SetValues = []string{fmt.Sprintf("pgEdge.initSpockImageName=%s", initSpockImg)}
	}
	return testHelm.Upgrade(helmRelease, opts)
}

func uninstallChart(t *testing.T) {
	t.Helper()
	if err := testHelm.Uninstall(helmRelease); err != nil {
		t.Logf("helm uninstall warning: %v", err)
	}
}

func getPodName(clusterName string) string {
	return clusterName + "-1"
}
