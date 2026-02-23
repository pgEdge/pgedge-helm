package helm

import (
	"fmt"
	"os/exec"
	"strings"
)

// Helm wraps the helm CLI.
type Helm struct {
	KubeContext string
	Namespace   string
}

// InstallOpts configures a helm install or upgrade.
type InstallOpts struct {
	ChartRef    string   // local path, OCI URI, or repo/chart
	Version     string   // --version flag (empty = omit)
	ValuesFiles []string // -f flags
	SetValues   []string // --set key=value
	Wait        bool
	Timeout     string // --timeout flag (e.g. "10m"), requires Wait
}

// UpgradeOpts configures a helm upgrade (same shape as InstallOpts).
type UpgradeOpts = InstallOpts

func (h *Helm) baseArgs() []string {
	var args []string
	if h.KubeContext != "" {
		args = append(args, "--kube-context", h.KubeContext)
	}
	if h.Namespace != "" {
		args = append(args, "--namespace", h.Namespace)
	}
	return args
}

// Template runs helm template and returns the rendered YAML.
func (h *Helm) Template(releaseName, chartRef string, valuesFiles ...string) (string, error) {
	args := append(h.baseArgs(), "template", releaseName, chartRef)
	for _, f := range valuesFiles {
		args = append(args, "-f", f)
	}
	return h.run(args...)
}

// Install runs helm install.
func (h *Helm) Install(releaseName string, opts InstallOpts) error {
	args := append(h.baseArgs(), "install", releaseName, opts.ChartRef)
	args = h.appendOpts(args, opts)
	_, err := h.run(args...)
	return err
}

// Upgrade runs helm upgrade.
func (h *Helm) Upgrade(releaseName string, opts UpgradeOpts) error {
	args := append(h.baseArgs(), "upgrade", releaseName, opts.ChartRef)
	args = h.appendOpts(args, opts)
	_, err := h.run(args...)
	return err
}

// Uninstall runs helm uninstall.
func (h *Helm) Uninstall(releaseName string) error {
	args := append(h.baseArgs(), "uninstall", releaseName)
	_, err := h.run(args...)
	return err
}

// RepoAdd runs helm repo add and helm repo update.
func (h *Helm) RepoAdd(name, url string) error {
	if _, err := h.run("repo", "add", name, url); err != nil {
		return err
	}
	_, err := h.run("repo", "update", name)
	return err
}

// Lint runs helm lint.
func (h *Helm) Lint(chartPath string, valuesFiles ...string) error {
	args := []string{"lint", chartPath}
	for _, f := range valuesFiles {
		args = append(args, "-f", f)
	}
	_, err := h.run(args...)
	return err
}

func (h *Helm) appendOpts(args []string, opts InstallOpts) []string {
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	for _, f := range opts.ValuesFiles {
		args = append(args, "-f", f)
	}
	for _, s := range opts.SetValues {
		args = append(args, "--set", s)
	}
	if opts.Wait {
		args = append(args, "--wait")
		if opts.Timeout != "" {
			args = append(args, "--timeout", opts.Timeout)
		}
	}
	return args
}

func (h *Helm) run(args ...string) (string, error) {
	cmd := exec.Command("helm", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm %s failed: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}
