package matchbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/homelab/pxe-in-a-box/internal/config"
)

func TestProfileID(t *testing.T) {
	got := ProfileID("talos-v1.10.6", "worker01")
	want := "talos-v1.10.6-worker01"
	if got != want {
		t.Errorf("ProfileID() = %q, want %q", got, want)
	}
}

func TestEndpoint_BaseURL(t *testing.T) {
	ep := Endpoint{Address: "192.168.2.103", Port: 8081}
	got := ep.BaseURL()
	want := "http://192.168.2.103:8081"
	if got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

func TestAssetBootPaths_Talos(t *testing.T) {
	info := &config.AssetInfo{ID: "talos-v1.10.6", OSType: config.OSTypeTalos, Arch: "amd64"}
	kernel, initrd, err := assetBootPaths(info)
	if err != nil {
		t.Fatalf("assetBootPaths failed: %v", err)
	}
	if kernel != "/assets/talos-v1.10.6/amd64/vmlinuz" {
		t.Errorf("kernel = %q", kernel)
	}
	if initrd != "/assets/talos-v1.10.6/amd64/initramfs.xz" {
		t.Errorf("initrd = %q", initrd)
	}
}

func TestAssetBootPaths_Ubuntu(t *testing.T) {
	info := &config.AssetInfo{ID: "ubuntu-noble", OSType: config.OSTypeUbuntu, Arch: "amd64"}
	kernel, initrd, err := assetBootPaths(info)
	if err != nil {
		t.Fatalf("assetBootPaths failed: %v", err)
	}
	if kernel != "/assets/ubuntu-noble/amd64/linux" {
		t.Errorf("kernel = %q", kernel)
	}
	if initrd != "/assets/ubuntu-noble/amd64/initrd" {
		t.Errorf("initrd = %q", initrd)
	}
}

func TestGenerateGroups(t *testing.T) {
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "workers",
				Profile:  "talos-v1.10.6",
				Template: "worker.yaml.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "worker01"},
					{MAC: config.MAC("00:e0:4c:68:00:0e"), Hostname: "worker02"},
				},
			},
		},
	}

	outputDir := t.TempDir()
	if err := GenerateGroups(machines, outputDir); err != nil {
		t.Fatalf("GenerateGroups failed: %v", err)
	}

	// Should have 2 group files
	files, err := filepath.Glob(filepath.Join(outputDir, "*.json"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 group files, got %d", len(files))
	}

	// Verify worker01 group
	groupPath := filepath.Join(outputDir, "worker01.json")
	data, err := os.ReadFile(groupPath)
	if err != nil {
		t.Fatalf("reading group file: %v", err)
	}

	var group GroupFile
	if err := json.Unmarshal(data, &group); err != nil {
		t.Fatalf("unmarshaling group: %v", err)
	}

	if group.ID != "worker01" {
		t.Errorf("group ID = %q, want 'worker01'", group.ID)
	}
	if group.Profile != "talos-v1.10.6-worker01" {
		t.Errorf("group Profile = %q, want 'talos-v1.10.6-worker01'", group.Profile)
	}
	if group.Selector["mac"] != "00:e0:4c:68:00:a1" {
		t.Errorf("group Selector mac = %q", group.Selector["mac"])
	}
}

func TestGenerateProfiles(t *testing.T) {
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "workers",
				Profile:  "talos-v1.10.6",
				Template: "worker.yaml.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "worker01"},
				},
			},
		},
		Singletons: []config.Singleton{
			{
				MAC:      config.MAC("b8:ae:ed:73:c3:bc"),
				Hostname: "melfina",
				Profile:  "talos-v1.9.1",
				Config:   "melfina.yaml",
			},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
			{ID: "talos-v1.9.1", Version: "v1.9.1", Arch: "amd64"},
		},
	}

	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputDir := t.TempDir()

	if err := GenerateProfiles(machines, assets, endpoint, outputDir); err != nil {
		t.Fatalf("GenerateProfiles failed: %v", err)
	}

	// Verify group machine profile
	profilePath := filepath.Join(outputDir, "talos-v1.10.6-worker01.json")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("reading profile file: %v", err)
	}

	var profile ProfileFile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshaling profile: %v", err)
	}

	if profile.ID != "talos-v1.10.6-worker01" {
		t.Errorf("profile ID = %q", profile.ID)
	}
	if profile.Boot.Kernel != "/assets/talos-v1.10.6/amd64/vmlinuz" {
		t.Errorf("kernel = %q", profile.Boot.Kernel)
	}
	if len(profile.Boot.Initrd) != 1 || profile.Boot.Initrd[0] != "/assets/talos-v1.10.6/amd64/initramfs.xz" {
		t.Errorf("initrd = %v", profile.Boot.Initrd)
	}

	// Check talos.config URL points to rendered config for group machine
	foundConfigArg := false
	for _, arg := range profile.Boot.Args {
		if arg == "talos.config=http://192.168.2.103:8081/assets/rendered/worker01.yaml" {
			foundConfigArg = true
		}
	}
	if !foundConfigArg {
		t.Errorf("talos.config arg not found or incorrect in: %v", profile.Boot.Args)
	}

	// Verify singleton profile points to static config
	singletonPath := filepath.Join(outputDir, "talos-v1.9.1-melfina.json")
	singletonData, err := os.ReadFile(singletonPath)
	if err != nil {
		t.Fatalf("reading singleton profile: %v", err)
	}

	var singletonProfile ProfileFile
	if err := json.Unmarshal(singletonData, &singletonProfile); err != nil {
		t.Fatalf("unmarshaling singleton profile: %v", err)
	}

	foundStaticConfig := false
	for _, arg := range singletonProfile.Boot.Args {
		if arg == "talos.config=http://192.168.2.103:8081/assets/static/melfina.yaml" {
			foundStaticConfig = true
		}
	}
	if !foundStaticConfig {
		t.Errorf("talos.config for singleton should point to static/, got: %v", singletonProfile.Boot.Args)
	}
}

func TestGenerateGroups_ClearsStaleFiles(t *testing.T) {
	outputDir := t.TempDir()

	// Write a stale file
	stalePath := filepath.Join(outputDir, "old-machine.json")
	if err := os.WriteFile(stalePath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "g",
				Profile:  "talos-v1.10.6",
				Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "new-machine"},
				},
			},
		},
	}

	if err := GenerateGroups(machines, outputDir); err != nil {
		t.Fatalf("GenerateGroups failed: %v", err)
	}

	// Stale file should be gone
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("stale file should have been removed")
	}

	// New file should exist
	newPath := filepath.Join(outputDir, "new-machine.json")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new group file should exist: %v", err)
	}
}

func TestGenerateProfiles_AssetNotFound(t *testing.T) {
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "g",
				Profile:  "nonexistent",
				Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:e0:4c:68:00:a1"), Hostname: "m1"},
				},
			},
		},
	}

	assets := &config.AssetsConfig{}
	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputDir := t.TempDir()

	err := GenerateProfiles(machines, assets, endpoint, outputDir)
	if err == nil {
		t.Fatal("expected error for missing asset")
	}
}
