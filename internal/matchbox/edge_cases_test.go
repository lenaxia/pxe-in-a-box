package matchbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/config"
)

func TestGenerateProfiles_MixedGroupsAndSingletons(t *testing.T) {
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "workers", Profile: "talos-v1.10.6", Template: "worker.yaml.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:00:00:00:00:a1"), Hostname: "worker01"},
					{MAC: config.MAC("00:00:00:00:00:a2"), Hostname: "worker02"},
				},
			},
		},
		Singletons: []config.Singleton{
			{MAC: config.MAC("00:00:00:00:00:b1"), Hostname: "special", Profile: "talos-v1.9.1", Config: "special.yaml"},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
			{ID: "talos-v1.9.1", Version: "v1.9.1", Arch: "amd64"},
		},
	}

	endpoint := Endpoint{Address: "10.0.0.1", Port: 8081}
	outputDir := t.TempDir()

	if err := GenerateProfiles(machines, assets, endpoint, outputDir); err != nil {
		t.Fatalf("GenerateProfiles: %v", err)
	}

	// All 3 machines should produce profiles
	expected := []string{
		"talos-v1.10.6-worker01.json",
		"talos-v1.10.6-worker02.json",
		"talos-v1.9.1-special.json",
	}
	for _, name := range expected {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("expected profile %s to exist: %v", name, err)
		}
	}

	// Group machines point to rendered/, singleton points to static/
	workerProfile := readProfileFile(t, filepath.Join(outputDir, "talos-v1.10.6-worker01.json"))
	singletonProfile := readProfileFile(t, filepath.Join(outputDir, "talos-v1.9.1-special.json"))

	foundRendered := false
	foundStatic := false
	for _, arg := range workerProfile.Boot.Args {
		if len(arg) > 20 && arg[len(arg)-len("/assets/rendered/worker01.yaml"):] == "/assets/rendered/worker01.yaml" {
			foundRendered = true
		}
	}
	for _, arg := range singletonProfile.Boot.Args {
		if len(arg) > 20 && arg[len(arg)-len("/assets/static/special.yaml"):] == "/assets/static/special.yaml" {
			foundStatic = true
		}
	}
	if !foundRendered {
		t.Error("group machine should point to rendered/")
	}
	if !foundStatic {
		t.Error("singleton should point to static/")
	}
}

func TestGenerateGroups_LargeGroup(t *testing.T) {
	machines := []config.GroupMachine{}
	for i := 0; i < 20; i++ {
		machines = append(machines, config.GroupMachine{
			MAC:      config.MAC("00:00:00:00:00:" + string(rune('a'+i))),
			Hostname: "worker" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
		})
	}

	cfg := &config.MachinesConfig{
		Groups: []config.Group{
			{Name: "big", Profile: "talos-v1.10.6", Template: "t.j2", Machines: machines},
		},
	}

	outputDir := t.TempDir()
	if err := GenerateGroups(cfg, outputDir); err != nil {
		t.Fatalf("GenerateGroups: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(outputDir, "*.json"))
	if len(files) != 20 {
		t.Errorf("expected 20 group files, got %d", len(files))
	}
}

func TestAssetBootPaths_UnknownOSType(t *testing.T) {
	info := &config.AssetInfo{
		ID:     "unknown",
		OSType: config.OSType("nonexistent"),
		Arch:   "amd64",
	}

	_, _, err := assetBootPaths(info)
	if err == nil {
		t.Fatal("expected error for unknown OS type")
	}
}

func TestAssetBootPaths_AllOSTypes(t *testing.T) {
	tests := []struct {
		osType     config.OSType
		wantKernel string
		wantInitrd string
	}{
		{config.OSTypeTalos, "/assets/test/amd64/vmlinuz", "/assets/test/amd64/initramfs.xz"},
		{config.OSTypeUbuntu, "/assets/test/amd64/linux", "/assets/test/amd64/initrd"},
		{config.OSTypeDebian, "/assets/test/amd64/linux", "/assets/test/amd64/initrd.gz"},
		{config.OSTypeArch, "/assets/test/amd64/vmlinuz-linux", "/assets/test/amd64/initramfs-linux.img"},
	}

	for _, tt := range tests {
		t.Run(string(tt.osType), func(t *testing.T) {
			info := &config.AssetInfo{ID: "test", OSType: tt.osType, Arch: "amd64"}
			kernel, initrd, err := assetBootPaths(info)
			if err != nil {
				t.Fatalf("assetBootPaths: %v", err)
			}
			if kernel != tt.wantKernel {
				t.Errorf("kernel = %q, want %q", kernel, tt.wantKernel)
			}
			if initrd != tt.wantInitrd {
				t.Errorf("initrd = %q, want %q", initrd, tt.wantInitrd)
			}
		})
	}
}

func TestGenerateProfiles_PerMachineConfigURLs(t *testing.T) {
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:00:00:00:00:01"), Hostname: "node-a"},
					{MAC: config.MAC("00:00:00:00:00:02"), Hostname: "node-b"},
					{MAC: config.MAC("00:00:00:00:00:03"), Hostname: "node-c"},
				},
			},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}

	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputDir := t.TempDir()

	if err := GenerateProfiles(machines, assets, endpoint, outputDir); err != nil {
		t.Fatalf("GenerateProfiles: %v", err)
	}

	// Each machine must have its OWN unique talos.config URL
	for _, hostname := range []string{"node-a", "node-b", "node-c"} {
		p := readProfileFile(t, filepath.Join(outputDir, "talos-v1.10.6-"+hostname+".json"))

		wantURL := "talos.config=http://192.168.2.103:8081/assets/rendered/" + hostname + ".yaml"
		found := false
		for _, arg := range p.Boot.Args {
			if arg == wantURL {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected arg %q not found in %v", hostname, wantURL, p.Boot.Args)
		}

		// Ensure OTHER hostnames' URLs are NOT present
		for _, otherHost := range []string{"node-a", "node-b", "node-c"} {
			if otherHost == hostname {
				continue
			}
			otherURL := "rendered/" + otherHost + ".yaml"
			for _, arg := range p.Boot.Args {
				if len(arg) > len(otherURL) && arg[len(arg)-len(otherURL):] == otherURL {
					t.Errorf("%s: should NOT contain %q", hostname, otherURL)
				}
			}
		}
	}
}

func readProfileFile(t *testing.T, path string) *ProfileFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var p ProfileFile
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return &p
}
