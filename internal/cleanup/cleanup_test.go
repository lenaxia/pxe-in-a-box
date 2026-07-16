package cleanup

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/config"
)

func newTestCleaner(t *testing.T) (*Cleaner, string) {
	dir := t.TempDir()
	return &Cleaner{
		AssetsDir: dir,
		Log:       log.New(os.Stderr, "", 0),
	}, dir
}

func TestRun_DeletesOrphanedAssets(t *testing.T) {
	c, dir := newTestCleaner(t)

	// Create a valid asset dir
	os.MkdirAll(filepath.Join(dir, "talos-v1.10.6", "amd64"), 0755)
	os.WriteFile(filepath.Join(dir, "talos-v1.10.6", "amd64", "vmlinuz"), []byte("kernel"), 0644)

	// Create an orphaned asset dir
	os.MkdirAll(filepath.Join(dir, "talos-v1.9.0", "amd64"), 0755)
	os.WriteFile(filepath.Join(dir, "talos-v1.9.0", "amd64", "vmlinuz"), []byte("old kernel"), 0644)

	cfg := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
	}

	results := c.Run(cfg)

	if len(results) != 1 {
		t.Fatalf("expected 1 deletion, got %d", len(results))
	}
	if results[0].Path != filepath.Join(dir, "talos-v1.9.0") {
		t.Errorf("expected talos-v1.9.0 deleted, got %s", results[0].Path)
	}

	// Verify orphaned dir is gone
	if _, err := os.Stat(filepath.Join(dir, "talos-v1.9.0")); !os.IsNotExist(err) {
		t.Error("orphaned dir should be deleted")
	}

	// Verify valid dir still exists
	if _, err := os.Stat(filepath.Join(dir, "talos-v1.10.6")); err != nil {
		t.Error("valid dir should still exist")
	}
}

func TestRun_ProtectsRenderedAndBoot(t *testing.T) {
	c, dir := newTestCleaner(t)

	// Create protected paths with content
	os.MkdirAll(filepath.Join(dir, "rendered"), 0755)
	os.WriteFile(filepath.Join(dir, "rendered", "worker01.yaml"), []byte("config"), 0644)
	os.WriteFile(filepath.Join(dir, "boot.ipxe"), []byte("#!ipxe"), 0644)
	os.MkdirAll(filepath.Join(dir, "static"), 0755)

	// Manifest with no assets (everything is orphaned except protected)
	cfg := &config.AssetsConfig{}

	results := c.Run(cfg)

	for _, r := range results {
		if r.Path == filepath.Join(dir, "rendered") {
			t.Error("rendered/ should be protected")
		}
		if r.Path == filepath.Join(dir, "boot.ipxe") {
			t.Error("boot.ipxe should be protected")
		}
		if r.Path == filepath.Join(dir, "static") {
			t.Error("static/ should be protected")
		}
	}
}

func TestRun_DryRun(t *testing.T) {
	c, dir := newTestCleaner(t)
	c.DryRun = true

	// Create an orphaned dir
	os.MkdirAll(filepath.Join(dir, "orphan"), 0755)
	os.WriteFile(filepath.Join(dir, "orphan", "file"), []byte("data"), 0644)

	cfg := &config.AssetsConfig{}

	results := c.Run(cfg)

	if len(results) != 1 {
		t.Fatalf("expected 1 dry-run result, got %d", len(results))
	}

	// File should still exist
	if _, err := os.Stat(filepath.Join(dir, "orphan")); err != nil {
		t.Error("dry-run should not delete files")
	}
}

func TestRun_EmptyAssetsDir(t *testing.T) {
	c, _ := newTestCleaner(t)
	cfg := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
	}

	results := c.Run(cfg)
	if len(results) != 0 {
		t.Errorf("expected 0 deletions on empty dir, got %d", len(results))
	}
}

func TestShouldCleanup(t *testing.T) {
	tests := []struct {
		cfgConfig bool
		cliFlag   bool
		want      bool
	}{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	}

	for _, tt := range tests {
		cfg := &config.AssetsConfig{Cleanup: tt.cfgConfig}
		got := ShouldCleanup(cfg, tt.cliFlag)
		if got != tt.want {
			t.Errorf("ShouldCleanup(cfg=%v, cli=%v) = %v, want %v", tt.cfgConfig, tt.cliFlag, got, tt.want)
		}
	}
}

func TestIsProtected(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"rendered", true},
		{"static", true},
		{"boot.ipxe", true},
		{"talos-v1.10.6", false},
		{"ubuntu-noble", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsProtected(tt.name); got != tt.want {
				t.Errorf("IsProtected(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
