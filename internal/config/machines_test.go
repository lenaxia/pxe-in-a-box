package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file %s: %v", name, err)
	}
	return path
}

func TestLoadMachines_Valid(t *testing.T) {
	yaml := `
groups:
  - name: controlplane
    profile: talos-v1.10.6
    template: controlplane.yaml.j2
    vars:
      cluster_name: homelab
      cluster_endpoint: 192.168.1.10
    machines:
      - mac: 00:e0:4c:68:00:8e
        hostname: cp00
        vars:
          node_ip: 192.168.1.11
      - mac: F4:4D:30:68:A3:B3
        hostname: cp01

  - name: workers
    profile: talos-v1.10.6
    template: worker.yaml.j2
    machines:
      - mac: 00:e0:4c:68:00:a1
        hostname: worker01

singletons:
  - mac: b8:ae:ed:73:c3:bc
    hostname: melfina
    profile: talos-v1.9.1
    config: melfina.yaml
`
	path := writeTempFile(t, "machines.yaml", yaml)
	cfg, err := LoadMachines(path)
	if err != nil {
		t.Fatalf("LoadMachines failed: %v", err)
	}

	if len(cfg.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(cfg.Groups))
	}
	if cfg.Groups[0].Name != "controlplane" {
		t.Errorf("expected group name 'controlplane', got %q", cfg.Groups[0].Name)
	}
	if len(cfg.Groups[0].Machines) != 2 {
		t.Fatalf("expected 2 machines in controlplane, got %d", len(cfg.Groups[0].Machines))
	}

	// MAC normalization: uppercase input should be normalized to lowercase
	mac0 := cfg.Groups[0].Machines[0].MAC.String()
	if mac0 != "00:e0:4c:68:00:8e" {
		t.Errorf("expected normalized MAC '00:e0:4c:68:00:8e', got %q", mac0)
	}

	mac1 := cfg.Groups[0].Machines[1].MAC.String()
	if mac1 != "f4:4d:30:68:a3:b3" {
		t.Errorf("expected normalized MAC 'f4:4d:30:68:a3:b3', got %q", mac1)
	}

	if len(cfg.Singletons) != 1 {
		t.Fatalf("expected 1 singleton, got %d", len(cfg.Singletons))
	}
	if cfg.Singletons[0].Hostname != "melfina" {
		t.Errorf("expected singleton hostname 'melfina', got %q", cfg.Singletons[0].Hostname)
	}
}

func TestLoadMachines_InvalidMAC(t *testing.T) {
	yaml := `
groups:
  - name: test
    profile: talos-v1.10.6
    template: worker.yaml.j2
    machines:
      - mac: not-a-mac-address
        hostname: test01
`
	path := writeTempFile(t, "machines.yaml", yaml)
	_, err := LoadMachines(path)
	if err == nil {
		t.Fatal("expected error for invalid MAC, got nil")
	}
}

func TestLoadMachines_EmptyFile(t *testing.T) {
	path := writeTempFile(t, "machines.yaml", "")
	cfg, err := LoadMachines(path)
	if err != nil {
		t.Fatalf("LoadMachines on empty file should succeed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.AllMachines()) != 0 {
		t.Fatalf("expected 0 machines from empty config, got %d", len(cfg.AllMachines()))
	}
}

func TestLoadMachines_FileNotFound(t *testing.T) {
	_, err := LoadMachines("/nonexistent/machines.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestMachinesConfig_AllMachines(t *testing.T) {
	cfg := &MachinesConfig{
		Groups: []Group{
			{
				Name:     "cp",
				Profile:  "talos-v1.10.6",
				Template: "cp.yaml.j2",
				Vars:     map[string]string{"cluster": "homelab"},
				Machines: []GroupMachine{
					{MAC: mustMAC(t, "00:e0:4c:68:00:8e"), Hostname: "cp00"},
					{
						MAC:      mustMAC(t, "00:e0:4c:68:00:8f"),
						Hostname: "cp01",
						Vars:     map[string]string{"node_ip": "10.0.0.2"},
					},
				},
			},
		},
		Singletons: []Singleton{
			{
				MAC:      mustMAC(t, "b8:ae:ed:73:c3:bc"),
				Hostname: "melfina",
				Profile:  "talos-v1.9.1",
				Config:   "melfina.yaml",
			},
		},
	}

	machines := cfg.AllMachines()
	if len(machines) != 3 {
		t.Fatalf("expected 3 machines, got %d", len(machines))
	}

	// Group machines should have merged vars
	cp01 := machines[1]
	if cp01.Vars["cluster"] != "homelab" {
		t.Errorf("expected merged var cluster=homelab, got %q", cp01.Vars["cluster"])
	}
	if cp01.Vars["node_ip"] != "10.0.0.2" {
		t.Errorf("expected machine var node_ip=10.0.0.2, got %q", cp01.Vars["node_ip"])
	}

	// Singleton should have no template, have config
	melfina := machines[2]
	if melfina.Template != "" {
		t.Errorf("expected empty template for singleton, got %q", melfina.Template)
	}
	if melfina.Config != "melfina.yaml" {
		t.Errorf("expected config 'melfina.yaml', got %q", melfina.Config)
	}
	if melfina.Source != "singleton" {
		t.Errorf("expected source 'singleton', got %q", melfina.Source)
	}
}

func mustMAC(t *testing.T, s string) MAC {
	t.Helper()
	mac, err := NewMAC(s)
	if err != nil {
		t.Fatalf("NewMAC(%q): %v", s, err)
	}
	return mac
}
