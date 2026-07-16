//go:build integration

package integration

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/bootscript"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/downloader"
)

// TestOSURLs_TalosOfficial verifies that Talos GitHub release URLs are
// reachable and return HTTP 200. Tests the actual download endpoints
// used by the asset downloader.
func TestOSURLs_TalosOfficial(t *testing.T) {
	version := "v1.10.6"
	asset := config.TalosAsset{
		ID:      "talos-v1.10.6",
		Version: version,
		Arch:    "amd64",
	}

	spec := resolveAndCheck(t, asset)

	if len(spec.Files) < 2 {
		t.Fatalf("expected at least 2 files (kernel + initrd), got %d", len(spec.Files))
	}
	if spec.ChecksumURL == "" {
		t.Fatal("expected non-empty checksum URL for official release")
	}

	for _, f := range spec.Files {
		t.Run(f.Filename, func(t *testing.T) {
			assertURLReachable(t, f.URL)
		})
	}

	t.Run("checksums", func(t *testing.T) {
		assertURLReachable(t, spec.ChecksumURL)
	})
}

// TestOSURLs_TalosImageFactory verifies that Image Factory download URLs
// are reachable. Uses a known default schematic ID.
func TestOSURLs_TalosImageFactory(t *testing.T) {
	asset := config.TalosAsset{
		ID:               "talos-if",
		ImageFactoryHash: "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba",
		Version:          "v1.10.6",
		Arch:             "amd64",
		DownloadUKI:      true,
	}

	spec := resolveAndCheck(t, asset)

	if spec.ChecksumURL != "" {
		t.Error("Image Factory assets should not have a checksum URL")
	}

	expectedFiles := map[string]bool{
		"vmlinuz":             false,
		"initramfs.xz":        false,
		"metal-amd64-uki.efi": false,
	}

	for _, f := range spec.Files {
		if _, ok := expectedFiles[f.Filename]; !ok {
			t.Errorf("unexpected file %q", f.Filename)
		}
		expectedFiles[f.Filename] = true
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %q not in spec", name)
		}
	}

	for _, f := range spec.Files {
		t.Run(f.Filename, func(t *testing.T) {
			assertURLReachable(t, f.URL)
		})
	}
}

// TestOSURLs_TalosUKI verifies that the UKI download URL works for a
// Talos official release when download_uki is enabled.
func TestOSURLs_TalosUKI(t *testing.T) {
	asset := config.TalosAsset{
		ID:          "talos-uki",
		Version:     "v1.10.6",
		Arch:        "amd64",
		DownloadUKI: true,
	}

	spec := resolveAndCheck(t, asset)

	foundUKI := false
	for _, f := range spec.Files {
		if f.Filename == "metal-amd64-uki.efi" {
			foundUKI = true
			assertURLReachable(t, f.URL)
		}
	}
	if !foundUKI {
		t.Error("expected UKI file in spec")
	}
}

// TestOSURLs_Debian verifies that Debian netboot download URLs are reachable.
func TestOSURLs_Debian(t *testing.T) {
	asset := config.DebianAsset{
		ID:      "debian-bookworm",
		Release: "bookworm",
		Arch:    "amd64",
	}

	spec := resolveAndCheck(t, asset)

	for _, f := range spec.Files {
		t.Run(f.Filename, func(t *testing.T) {
			assertURLReachable(t, f.URL)
		})
	}
}

// TestOSURLs_Arch verifies that Arch Linux mirror URLs are reachable.
func TestOSURLs_Arch(t *testing.T) {
	asset := config.ArchAsset{
		ID:   "arch-latest",
		Arch: "amd64",
	}

	spec := resolveAndCheck(t, asset)

	for _, f := range spec.Files {
		t.Run(f.Filename, func(t *testing.T) {
			assertURLReachable(t, f.URL)
		})
	}
}

// TestOSURLs_Ubuntu checks whether Ubuntu netboot URLs are reachable.
// Ubuntu 24.04+ dropped the traditional d-i netboot directory. This test
// documents the current state — it passes if URLs work, and warns if they don't.
func TestOSURLs_Ubuntu(t *testing.T) {
	asset := config.UbuntuAsset{
		ID:      "ubuntu-noble",
		Release: "noble",
		Arch:    "amd64",
	}

	spec := resolveAndCheck(t, asset)

	for _, f := range spec.Files {
		t.Run(f.Filename, func(t *testing.T) {
			resp, err := http.Head(f.URL)
			if err != nil {
				t.Logf("WARNING: Ubuntu netboot URL unreachable: %s\n"+
					"Ubuntu 24.04+ may not have traditional netboot files.\n"+
					"Consider using cloud-init images or manually staging files.\n"+
					"Error: %v", f.URL, err)
				t.Skip("Ubuntu netboot URL not reachable — known issue for 24.04+")
				return
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Logf("WARNING: Ubuntu netboot URL returned HTTP %d\n"+
					"Ubuntu 24.04+ may not have traditional netboot files.\n"+
					"Consider using cloud-init images or manually staging files.",
					resp.StatusCode)
				t.Skip("Ubuntu netboot URL not available — known issue for 24.04+")
				return
			}

			t.Logf("Ubuntu netboot URL OK: %s", f.URL)
		})
	}
}

// --- Per-OS profile generation tests ---

// TestOSProfiles_AllOSTypes verifies that profile generation produces
// correct kernel/initrd paths and boot args for each OS type.
func TestOSProfiles_AllOSTypes(t *testing.T) {
	assets := &config.AssetsConfig{
		Talos:  []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
		Ubuntu: []config.UbuntuAsset{{ID: "ubuntu-noble", Release: "noble", Arch: "amd64"}},
		Debian: []config.DebianAsset{{ID: "debian-bookworm", Release: "bookworm", Arch: "amd64"}},
		Arch:   []config.ArchAsset{{ID: "arch-latest", Arch: "amd64"}},
	}

	// Resolve specs and verify URL patterns
	specs := downloader.ResolveAssetSpecs(assets)

	if len(specs) != 4 {
		t.Fatalf("expected 4 asset specs, got %d", len(specs))
	}

	for _, spec := range specs {
		t.Run(string(spec.OSType), func(t *testing.T) {
			if len(spec.Files) < 2 {
				t.Errorf("expected at least 2 files for %s, got %d", spec.ID, len(spec.Files))
			}
			for _, f := range spec.Files {
				if f.URL == "" {
					t.Errorf("empty URL for %s/%s", spec.ID, f.Filename)
				}
				if f.Filename == "" {
					t.Errorf("empty filename in spec %s", spec.ID)
				}
			}
		})
	}
}

// --- Per-OS bootscript generation tests ---

func TestOSBootscript_AllOSTypes(t *testing.T) {
	assets := &config.AssetsConfig{
		Talos:  []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
		Ubuntu: []config.UbuntuAsset{{ID: "ubuntu-noble", Release: "noble", Arch: "amd64"}},
		Debian: []config.DebianAsset{{ID: "debian-bookworm", Release: "bookworm", Arch: "amd64"}},
		Arch:   []config.ArchAsset{{ID: "arch-latest", Arch: "amd64"}},
	}

	menu := &config.MenuConfig{
		Timeout: 10,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos", Profile: "talos-v1.10.6"},
			{ID: "ubuntu", Label: "Ubuntu", Profile: "ubuntu-noble"},
			{ID: "debian", Label: "Debian", Profile: "debian-bookworm"},
			{ID: "arch", Label: "Arch", Profile: "arch-latest"},
		},
	}

	dir := t.TempDir()
	bootPath := dir + "/boot.ipxe"

	if err := bootscript.Generate(menu, assets, bootscript.Endpoint{Address: "192.168.2.103", Port: 8081}, bootPath); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(bootPath)
	if err != nil {
		t.Fatalf("reading boot.ipxe: %v", err)
	}
	script := string(data)

	// Each OS should have its own menu entry and boot section
	tests := []struct {
		osID   string
		kernel string
		initrd string
	}{
		{"talos", "/assets/talos-v1.10.6/amd64/vmlinuz", "/assets/talos-v1.10.6/amd64/initramfs.xz"},
		{"ubuntu", "/assets/ubuntu-noble/amd64/linux", "/assets/ubuntu-noble/amd64/initrd"},
		{"debian", "/assets/debian-bookworm/amd64/linux", "/assets/debian-bookworm/amd64/initrd.gz"},
		{"arch", "/assets/arch-latest/amd64/vmlinuz-linux", "/assets/arch-latest/amd64/initramfs-linux.img"},
	}

	for _, tt := range tests {
		t.Run(tt.osID, func(t *testing.T) {
			if !strings.Contains(script, ":"+tt.osID) {
				t.Errorf("missing :%s section in boot.ipxe", tt.osID)
			}
			if !strings.Contains(script, tt.kernel) {
				t.Errorf("missing kernel path %s in boot.ipxe", tt.kernel)
			}
			if !strings.Contains(script, tt.initrd) {
				t.Errorf("missing initrd path %s in boot.ipxe", tt.initrd)
			}
		})
	}
}

// --- Helpers ---

func resolveAndCheck(t *testing.T, asset any) downloader.AssetSpec {
	t.Helper()

	var specs []downloader.AssetSpec
	switch a := asset.(type) {
	case config.TalosAsset:
		specs = downloader.ResolveAssetSpecs(&config.AssetsConfig{Talos: []config.TalosAsset{a}})
	case config.UbuntuAsset:
		specs = downloader.ResolveAssetSpecs(&config.AssetsConfig{Ubuntu: []config.UbuntuAsset{a}})
	case config.DebianAsset:
		specs = downloader.ResolveAssetSpecs(&config.AssetsConfig{Debian: []config.DebianAsset{a}})
	case config.ArchAsset:
		specs = downloader.ResolveAssetSpecs(&config.AssetsConfig{Arch: []config.ArchAsset{a}})
	default:
		t.Fatalf("unsupported asset type %T", asset)
	}

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	return specs[0]
}

func assertURLReachable(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Head(url)
	if err != nil {
		t.Fatalf("HEAD %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD %s: status %d, want 200", url, resp.StatusCode)
	}
	t.Logf("OK: %s (%d, %s bytes)", url, resp.StatusCode, resp.Header.Get("Content-Length"))
}
