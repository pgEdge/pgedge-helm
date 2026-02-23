//go:build unit

package unit

import "testing"

func TestCertsProvisionedByDefault(t *testing.T) {
	objects := renderTemplate(t, "values-distributed.yaml")

	issuers := filterByKind(objects, "Issuer")
	if len(issuers) != 2 {
		t.Errorf("expected 2 Issuers, got %d", len(issuers))
	}

	certs := filterByKind(objects, "Certificate")
	// 5 certificates: client-ca, streaming-replica, pgedge, admin, app
	if len(certs) != 5 {
		t.Errorf("expected 5 Certificates, got %d", len(certs))
	}
}

func TestCertsSkippedWhenDisabled(t *testing.T) {
	objects := renderTemplate(t, "values-no-certs.yaml")

	issuers := filterByKind(objects, "Issuer")
	if len(issuers) != 0 {
		t.Errorf("expected 0 Issuers, got %d", len(issuers))
	}

	certs := filterByKind(objects, "Certificate")
	if len(certs) != 0 {
		t.Errorf("expected 0 Certificates, got %d", len(certs))
	}
}
