// internal/spock/version_test.go
package spock

import "testing"

func TestParseSpockMajorVersion(t *testing.T) {
	cases := map[string]uint64{
		"5.0.6":       5,
		"5":           5,
		"6.0.0":       6,
		"6":           6,
		"6.0.0-beta1": 6,
		"6-beta1":     6,
		"10.2.1":      10,
	}
	for input, want := range cases {
		got, err := parseSpockMajorVersion(input)
		if err != nil {
			t.Errorf("parseSpockMajorVersion(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseSpockMajorVersion(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseSpockMajorVersionInvalid(t *testing.T) {
	for _, input := range []string{"", "beta", "x.0.0"} {
		if _, err := parseSpockMajorVersion(input); err == nil {
			t.Errorf("parseSpockMajorVersion(%q) expected error, got nil", input)
		}
	}
}
