package config

import (
	"testing"
)

func TestLoadMenu_Valid(t *testing.T) {
	yaml := `
timeout: 10
default: talos-maintenance
entries:
  - id: talos-maintenance
    label: "Talos Linux (Maintenance Mode)"
    profile: talos-maintenance
  - id: ubuntu-noble
    label: "Ubuntu Server 24.04"
    profile: ubuntu-noble
`
	path := writeTempFile(t, "menu.yaml", yaml)
	cfg, err := LoadMenu(path)
	if err != nil {
		t.Fatalf("LoadMenu failed: %v", err)
	}

	if cfg.Timeout != 10 {
		t.Errorf("expected timeout 10, got %d", cfg.Timeout)
	}
	if cfg.Default != "talos-maintenance" {
		t.Errorf("expected default 'talos-maintenance', got %q", cfg.Default)
	}
	if len(cfg.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cfg.Entries))
	}
	if cfg.Entries[0].ID != "talos-maintenance" {
		t.Errorf("expected entry[0] id 'talos-maintenance', got %q", cfg.Entries[0].ID)
	}
}

func TestLoadMenu_Empty(t *testing.T) {
	path := writeTempFile(t, "menu.yaml", "")
	cfg, err := LoadMenu(path)
	if err != nil {
		t.Fatalf("LoadMenu on empty file should succeed: %v", err)
	}
	if cfg.Timeout != 0 {
		t.Errorf("expected default timeout 0, got %d", cfg.Timeout)
	}
}
