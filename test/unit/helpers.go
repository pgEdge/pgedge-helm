//go:build unit

package unit

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// chartPath returns the absolute path to the chart root (parent of test/).
func chartPath(t *testing.T) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// testdataPath returns the absolute path to a file in testdata/.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

// renderTemplate runs helm template with the given values file and returns parsed objects.
func renderTemplate(t *testing.T, valuesFile string) []unstructured.Unstructured {
	t.Helper()
	chart := chartPath(t)
	values := testdataPath(t, valuesFile)

	cmd := exec.Command("helm", "template", "pgedge", chart, "-f", values)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, string(out))
	}

	return parseYAMLDocuments(t, string(out))
}

// renderTemplateExpectError runs helm template expecting failure, returns stderr.
func renderTemplateExpectError(t *testing.T, valuesFile string) string {
	t.Helper()
	chart := chartPath(t)
	values := testdataPath(t, valuesFile)

	cmd := exec.Command("helm", "template", "pgedge", chart, "-f", values)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected helm template to fail, but it succeeded:\n%s", string(out))
	}
	return string(out)
}

// renderTemplateUpgradeExpectError runs helm template with --is-upgrade expecting failure.
func renderTemplateUpgradeExpectError(t *testing.T, valuesFile string) string {
	t.Helper()
	chart := chartPath(t)
	values := testdataPath(t, valuesFile)

	cmd := exec.Command("helm", "template", "pgedge", chart, "-f", values, "--is-upgrade")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected helm template --is-upgrade to fail, but it succeeded:\n%s", string(out))
	}
	return string(out)
}

// parseYAMLDocuments splits multi-document YAML and parses each into Unstructured.
func parseYAMLDocuments(t *testing.T, raw string) []unstructured.Unstructured {
	t.Helper()
	var objects []unstructured.Unstructured

	docs := strings.Split(raw, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096)
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			t.Fatalf("failed to parse YAML document: %v\n%s", err, doc)
		}
		if obj.GetKind() == "" {
			continue
		}
		objects = append(objects, *obj)
	}

	return objects
}

// filterByKind returns objects matching the given kind.
func filterByKind(objects []unstructured.Unstructured, kind string) []unstructured.Unstructured {
	var result []unstructured.Unstructured
	for _, obj := range objects {
		if obj.GetKind() == kind {
			result = append(result, obj)
		}
	}
	return result
}

// findByKindAndName returns the first object matching kind and name.
func findByKindAndName(objects []unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	for _, obj := range objects {
		if obj.GetKind() == kind && obj.GetName() == name {
			return &obj
		}
	}
	return nil
}

// getNestedString is a convenience wrapper for unstructured field access.
func getNestedString(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if err != nil || !found {
		return ""
	}
	return val
}
