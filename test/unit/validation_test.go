//go:build unit

package unit

import (
	"strings"
	"testing"
)

func TestAppNameTooLong(t *testing.T) {
	output := renderTemplateExpectError(t, "invalid-appname-values.yaml")
	if !strings.Contains(output, "appName") {
		t.Errorf("expected schema validation error about appName, got:\n%s", output)
	}
}

func TestNodesMissingHostname(t *testing.T) {
	output := renderTemplateExpectError(t, "invalid-no-hostname-values.yaml")
	if !strings.Contains(output, "hostname") {
		t.Errorf("expected schema validation error about missing hostname, got:\n%s", output)
	}
}

func TestNodesMissingName(t *testing.T) {
	output := renderTemplateExpectError(t, "invalid-no-name-values.yaml")
	if !strings.Contains(output, "name") {
		t.Errorf("expected schema validation error about missing name, got:\n%s", output)
	}
}

func TestNodesInvalidBootstrapMode(t *testing.T) {
	output := renderTemplateExpectError(t, "invalid-bootstrap-mode-values.yaml")
	if !strings.Contains(output, "mode") {
		t.Errorf("expected schema validation error about bootstrap.mode enum, got:\n%s", output)
	}
}

// Tests for validate.newNodes in _helpers.tpl.
// During helm template --is-upgrade, lookup returns empty so all nodes appear new.

func TestUpgradeNewNodeRequiresBootstrapMode(t *testing.T) {
	// A node without bootstrap.mode should fail on upgrade
	output := renderTemplateUpgradeExpectError(t, "single-node-minimal-values.yaml")
	if !strings.Contains(output, "must specify bootstrap.mode") {
		t.Errorf("expected 'must specify bootstrap.mode' error, got:\n%s", output)
	}
}
