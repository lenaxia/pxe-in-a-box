//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/bootscript"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/matchbox"
)

// TestFullPipeline_GroupProfileBootscript exercises the entire generation
// pipeline: load configs → validate → generate groups → generate profiles →
// generate boot.ipxe. Then verifies every output file.
func TestFullPipeline_GroupProfileBootscript(t *testing.T) {
	// --- Arrange: create input configs ---

	dir := t.TempDir()

	machinesYAML := `
groups:
  - name: controlplane
    profile: talos-v1.10.6
    template: controlplane.yaml.j2
    vars:
      cluster_name: homelab
      cluster_endpoint: 192.168.2.10
    machines:
      - mac: 00:e0:4c:68:00:8e
        hostname: cp00
        vars:
          node_ip: 192.168.2.11

  - name: workers
    profile: talos-v1.10.6
    template: worker.yaml.j2
    machines:
      - mac: 00:e0:4c:68:00:a1
        hostname: worker01
      - mac: 00:e0:4c:68:00:0e
        hostname: worker02

singletons:
  - mac: b8:ae:ed:73:c3:bc
    hostname: melfina
    profile: talos-v1.9.1
    config: melfina.yaml
`

	assetsYAML := `
talos:
  - id: talos-v1.10.6
    version: v1.10.6
    arch: amd64
  - id: talos-v1.9.1
    version: v1.9.1
    arch: amd64
ubuntu:
  - id: ubuntu-noble
    release: noble
    arch: amd64
`

	menuYAML := `
timeout: 10
default: talos
entries:
  - id: talos
    label: "Talos Linux (Maintenance Mode)"
    profile: talos-v1.10.6
  - id: ubuntu
    label: "Ubuntu Server 24.04"
    profile: ubuntu-noble
`

	machinesPath := writeT(t, dir, "machines.yaml", machinesYAML)
	assetsPath := writeT(t, dir, "assets.yaml", assetsYAML)
	menuPath := writeT(t, dir, "menu.yaml", menuYAML)

	// --- Act: load, validate, generate ---

	machines, err := config.LoadMachines(machinesPath)
	if err != nil {
		t.Fatalf("LoadMachines: %v", err)
	}

	assets, err := config.LoadAssets(assetsPath)
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}

	menu, err := config.LoadMenu(menuPath)
	if err != nil {
		t.Fatalf("LoadMenu: %v", err)
	}

	fc := &config.FullConfig{Machines: machines, Assets: assets, Menu: menu}
	if err := config.Validate(fc); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	endpoint := matchbox.Endpoint{Address: "192.168.2.103", Port: 8081}

	groupsDir := filepath.Join(dir, "groups")
	profilesDir := filepath.Join(dir, "profiles")
	bootPath := filepath.Join(dir, "assets", "boot.ipxe")

	if err := matchbox.GenerateGroups(machines, groupsDir); err != nil {
		t.Fatalf("GenerateGroups: %v", err)
	}
	if err := matchbox.GenerateProfiles(machines, assets, endpoint, profilesDir); err != nil {
		t.Fatalf("GenerateProfiles: %v", err)
	}
	if err := bootscript.Generate(menu, assets, bootscript.Endpoint(endpoint), bootPath); err != nil {
		t.Fatalf("Generate bootscript: %v", err)
	}

	// --- Assert: verify groups ---

	t.Run("groups", func(t *testing.T) {
		expected := []struct {
			filename string
			mac      string
			profile  string
		}{
			{"cp00.json", "00:e0:4c:68:00:8e", "talos-v1.10.6-cp00"},
			{"worker01.json", "00:e0:4c:68:00:a1", "talos-v1.10.6-worker01"},
			{"worker02.json", "00:e0:4c:68:00:0e", "talos-v1.10.6-worker02"},
			{"melfina.json", "b8:ae:ed:73:c3:bc", "talos-v1.9.1-melfina"},
		}

		for _, e := range expected {
			path := filepath.Join(groupsDir, e.filename)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("group file %s: %v", e.filename, err)
				continue
			}

			var group matchbox.GroupFile
			if err := json.Unmarshal(data, &group); err != nil {
				t.Errorf("unmarshal %s: %v", e.filename, err)
				continue
			}

			if group.Selector["mac"] != e.mac {
				t.Errorf("%s: mac = %q, want %q", e.filename, group.Selector["mac"], e.mac)
			}
			if group.Profile != e.profile {
				t.Errorf("%s: profile = %q, want %q", e.filename, group.Profile, e.profile)
			}
		}

		// Verify exactly 4 groups (no stale, no catch-all)
		files, _ := filepath.Glob(filepath.Join(groupsDir, "*.json"))
		if len(files) != 4 {
			t.Errorf("expected 4 group files, got %d", len(files))
		}
	})

	// --- Assert: verify profiles ---

	t.Run("profiles", func(t *testing.T) {
		// Group machine: talos.config points to rendered/
		p := readProfile(t, filepath.Join(profilesDir, "talos-v1.10.6-worker01.json"))
		if p.Boot.Kernel != "/assets/talos-v1.10.6/amd64/vmlinuz" {
			t.Errorf("kernel = %q", p.Boot.Kernel)
		}
		assertHasArg(t, p.Boot.Args, "talos.config=http://192.168.2.103:8081/assets/rendered/worker01.yaml")
		assertHasArg(t, p.Boot.Args, "talos.platform=metal")

		// Singleton: talos.config points to static/
		s := readProfile(t, filepath.Join(profilesDir, "talos-v1.9.1-melfina.json"))
		assertHasArg(t, s.Boot.Args, "talos.config=http://192.168.2.103:8081/assets/static/melfina.yaml")

		// Control plane machine: has node_ip in vars but that doesn't affect profile
		c := readProfile(t, filepath.Join(profilesDir, "talos-v1.10.6-cp00.json"))
		assertHasArg(t, c.Boot.Args, "talos.config=http://192.168.2.103:8081/assets/rendered/cp00.yaml")
	})

	// --- Assert: verify boot.ipxe ---

	t.Run("boot.ipxe", func(t *testing.T) {
		data, err := os.ReadFile(bootPath)
		if err != nil {
			t.Fatalf("reading boot.ipxe: %v", err)
		}
		script := string(data)

		assertContains(t, script, "chain http://192.168.2.103:8081/ipxe?mac=${mac:hexhyp}")
		assertContains(t, script, "|| goto menu")
		assertContains(t, script, ":menu")
		assertContains(t, script, "item talos Talos Linux (Maintenance Mode)")
		assertContains(t, script, "item ubuntu Ubuntu Server 24.04")
		assertContains(t, script, "choose --default talos --timeout 10000")
		assertContains(t, script, ":talos")
		assertContains(t, script, "talos.config=null")
		assertContains(t, script, ":ubuntu")
		assertContains(t, script, "/assets/ubuntu-noble/amd64/linux")
	})
}

// TestFullPipeline_RegenerationClearsStale verifies that re-running
// generation after removing a machine clears stale group/profile files.
func TestFullPipeline_RegenerationClearsStale(t *testing.T) {
	dir := t.TempDir()
	groupsDir := filepath.Join(dir, "groups")
	profilesDir := filepath.Join(dir, "profiles")

	// Initial config: 2 machines
	machines1 := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "m1"},
					{MAC: config.MAC("00:e0:4c:68:00:0e"), Hostname: "m2"},
				},
			},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}
	endpoint := matchbox.Endpoint{Address: "192.168.2.103", Port: 8081}

	matchbox.GenerateGroups(machines1, groupsDir)
	matchbox.GenerateProfiles(machines1, assets, endpoint, profilesDir)

	// Verify 2 files
	if files, _ := filepath.Glob(filepath.Join(groupsDir, "*.json")); len(files) != 2 {
		t.Fatalf("expected 2 groups initially, got %d", len(files))
	}

	// Second config: remove m2
	machines2 := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "m1"},
				},
			},
		},
	}

	matchbox.GenerateGroups(machines2, groupsDir)
	matchbox.GenerateProfiles(machines2, assets, endpoint, profilesDir)

	// Verify m2 is gone
	if _, err := os.Stat(filepath.Join(groupsDir, "m2.json")); !os.IsNotExist(err) {
		t.Error("stale group m2.json should be deleted")
	}
	if _, err := os.Stat(filepath.Join(profilesDir, "talos-v1.10.6-m2.json")); !os.IsNotExist(err) {
		t.Error("stale profile talos-v1.10.6-m2.json should be deleted")
	}

	// m1 still exists
	if _, err := os.Stat(filepath.Join(groupsDir, "m1.json")); err != nil {
		t.Error("m1.json should still exist")
	}
}

// TestFullPipeline_AllOSTypePaths verifies that profile generation produces
// correct kernel/initrd paths for each supported OS type.
func TestFullPipeline_AllOSTypePaths(t *testing.T) {
	machines := &config.MachinesConfig{
		Singletons: []config.Singleton{
			{MAC: config.MAC("00:00:00:00:00:01"), Hostname: "talos-node", Profile: "talos-v1.10.6", Config: "t.yaml"},
			{MAC: config.MAC("00:00:00:00:00:02"), Hostname: "ubuntu-node", Profile: "ubuntu-noble", Config: "u.yaml"},
			{MAC: config.MAC("00:00:00:00:00:03"), Hostname: "debian-node", Profile: "debian-bookworm", Config: "d.yaml"},
			{MAC: config.MAC("00:00:00:00:00:04"), Hostname: "arch-node", Profile: "arch-latest", Config: "a.yaml"},
		},
	}

	assets := &config.AssetsConfig{
		Talos:  []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
		Ubuntu: []config.UbuntuAsset{{ID: "ubuntu-noble", Release: "noble", Arch: "amd64"}},
		Debian: []config.DebianAsset{{ID: "debian-bookworm", Release: "bookworm", Arch: "amd64"}},
		Arch:   []config.ArchAsset{{ID: "arch-latest", Arch: "amd64"}},
	}

	endpoint := matchbox.Endpoint{Address: "10.0.0.1", Port: 8081}
	profilesDir := t.TempDir()

	if err := matchbox.GenerateProfiles(machines, assets, endpoint, profilesDir); err != nil {
		t.Fatalf("GenerateProfiles: %v", err)
	}

	tests := []struct {
		hostname string
		kernel   string
		initrd   string
	}{
		{"talos-node", "/assets/talos-v1.10.6/amd64/vmlinuz", "/assets/talos-v1.10.6/amd64/initramfs.xz"},
		{"ubuntu-node", "/assets/ubuntu-noble/amd64/linux", "/assets/ubuntu-noble/amd64/initrd"},
		{"debian-node", "/assets/debian-bookworm/amd64/linux", "/assets/debian-bookworm/amd64/initrd.gz"},
		{"arch-node", "/assets/arch-latest/amd64/vmlinuz-linux", "/assets/arch-latest/amd64/initramfs-linux.img"},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			// Find the profile file
			files, _ := filepath.Glob(filepath.Join(profilesDir, "*-"+tt.hostname+".json"))
			if len(files) != 1 {
				t.Fatalf("expected 1 profile for %s, got %d", tt.hostname, len(files))
			}

			p := readProfile(t, files[0])
			if p.Boot.Kernel != tt.kernel {
				t.Errorf("kernel = %q, want %q", p.Boot.Kernel, tt.kernel)
			}
			if len(p.Boot.Initrd) != 1 || p.Boot.Initrd[0] != tt.initrd {
				t.Errorf("initrd = %v, want [%q]", p.Boot.Initrd, tt.initrd)
			}
		})
	}
}

// --- Helpers ---

func writeT(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func readProfile(t *testing.T, path string) *matchbox.ProfileFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading profile %s: %v", path, err)
	}
	var p matchbox.ProfileFile
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshaling profile %s: %v", path, err)
	}
	return &p
}

func assertHasArg(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args %v do not contain %q", args, want)
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s[:min(200, len(s))], substr)
	}
}
