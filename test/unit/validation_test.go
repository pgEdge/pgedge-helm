//go:build unit

package unit

import (
	"strings"
	"testing"
)

func TestAppNameTooLong(t *testing.T) {
	output := renderTemplateExpectError(t, "values-invalid-appname.yaml")
	if !strings.Contains(output, "appName") || !strings.Contains(output, "maxLength") {
		t.Errorf("expected schema validation error about appName length, got:\n%s", output)
	}
}

func TestNodesMissingHostname(t *testing.T) {
	output := renderTemplateExpectError(t, "values-invalid-no-hostname.yaml")
	if !strings.Contains(output, "hostname") {
		t.Errorf("expected schema validation error about missing hostname, got:\n%s", output)
	}
}

func TestNodesMissingName(t *testing.T) {
	output := renderTemplateExpectError(t, "values-invalid-no-name.yaml")
	if !strings.Contains(output, "name") {
		t.Errorf("expected schema validation error about missing name, got:\n%s", output)
	}
}

func TestNodesInvalidBootstrapMode(t *testing.T) {
	output := renderTemplateExpectError(t, "values-invalid-bootstrap-mode.yaml")
	if !strings.Contains(output, "mode") {
		t.Errorf("expected schema validation error about bootstrap.mode enum, got:\n%s", output)
	}
}

// Tests for validate.newNodes in _helpers.tpl.
// During helm template --is-upgrade, lookup returns empty so all nodes appear new.

func TestUpgradeNewNodeRequiresBootstrapMode(t *testing.T) {
	// A node without bootstrap.mode should fail on upgrade
	output := renderTemplateUpgradeExpectError(t, "values-single-node-minimal.yaml")
	if !strings.Contains(output, "must specify bootstrap.mode") {
		t.Errorf("expected 'must specify bootstrap.mode' error, got:\n%s", output)
	}
}
