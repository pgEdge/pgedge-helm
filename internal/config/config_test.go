package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNodes(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
- name: n2
  hostname: pgedge-n2-rw
  internalHostname: pgedge-n2-rw.local
  bootstrap:
    mode: spock
    sourceNode: n1
`
	path := writeTemp(t, yaml)
	nodes, err := LoadNodes(path)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Name != "n1" || nodes[0].Hostname != "pgedge-n1-rw" {
		t.Errorf("n1: got %+v", nodes[0])
	}
	if nodes[0].InternalHostname != "" {
		t.Errorf("n1 should have no internalHostname, got %q", nodes[0].InternalHostname)
	}
	if nodes[0].Bootstrap.Mode != "" {
		t.Errorf("n1 should have empty bootstrap mode, got %q", nodes[0].Bootstrap.Mode)
	}
	if nodes[1].Name != "n2" || nodes[1].Hostname != "pgedge-n2-rw" {
		t.Errorf("n2: got %+v", nodes[1])
	}
	if nodes[1].InternalHostname != "pgedge-n2-rw.local" {
		t.Errorf("n2 internalHostname: got %q", nodes[1].InternalHostname)
	}
	if nodes[1].Bootstrap.Mode != "spock" || nodes[1].Bootstrap.SourceNode != "n1" {
		t.Errorf("n2 bootstrap: got %+v", nodes[1].Bootstrap)
	}
}

func TestLoadNodesEmpty(t *testing.T) {
	path := writeTemp(t, "")
	nodes, err := LoadNodes(path)
	if err != nil {
		t.Fatalf("LoadNodes empty: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestLoadNodesMissingFile(t *testing.T) {
	_, err := LoadNodes("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
`
	path := writeTemp(t, yaml)
	t.Setenv("APP_NAME", "pgedge")
	t.Setenv("DB_NAME", "app")
	t.Setenv("NAMESPACE", "test-ns")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppName != "pgedge" {
		t.Errorf("AppName: got %q", cfg.AppName)
	}
	if cfg.DBName != "app" {
		t.Errorf("DBName: got %q", cfg.DBName)
	}
	if cfg.Namespace != "test-ns" {
		t.Errorf("Namespace: got %q", cfg.Namespace)
	}
	if len(cfg.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(cfg.Nodes))
	}
}

func TestLoadConfigDefaultAdminUser(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
`
	path := writeTemp(t, yaml)
	t.Setenv("APP_NAME", "pgedge")
	t.Setenv("DB_NAME", "app")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("expected AdminUser=admin, got %q", cfg.AdminUser)
	}
}

func TestLoadConfigCustomAdminUser(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
`
	path := writeTemp(t, yaml)
	t.Setenv("APP_NAME", "pgedge")
	t.Setenv("DB_NAME", "app")
	t.Setenv("ADMIN_USER", "dbadmin")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminUser != "dbadmin" {
		t.Errorf("expected AdminUser=dbadmin, got %q", cfg.AdminUser)
	}
}

func TestLoadConfigResetSpock(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
`
	path := writeTemp(t, yaml)
	t.Setenv("APP_NAME", "pgedge")
	t.Setenv("DB_NAME", "app")
	t.Setenv("RESET_SPOCK", "true")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ResetSpock {
		t.Error("expected ResetSpock=true when RESET_SPOCK=true")
	}
}

func TestLoadConfigResetSpockDefault(t *testing.T) {
	yaml := `
- name: n1
  hostname: pgedge-n1-rw
`
	path := writeTemp(t, yaml)
	t.Setenv("APP_NAME", "pgedge")
	t.Setenv("DB_NAME", "app")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ResetSpock {
		t.Error("expected ResetSpock=false when RESET_SPOCK is unset")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pgedge.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
