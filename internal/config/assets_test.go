package config

import (
	"testing"
)

func TestLoadAssets_Valid(t *testing.T) {
	yaml := `
cleanup: false
talos:
  - id: talos-v1.10.6
    version: v1.10.6
    arch: amd64
  - id: talos-nvidia
    image_factory_hash: "37f5a3fbd1e1e5d2a3c4"
    version: v1.9.5
    arch: amd64
    download_uki: true
ubuntu:
  - id: ubuntu-noble
    release: noble
    arch: amd64
debian:
  - id: debian-bookworm
    release: bookworm
    arch: amd64
arch:
  - id: arch-latest
    arch: amd64
`
	path := writeTempFile(t, "assets.yaml", yaml)
	cfg, err := LoadAssets(path)
	if err != nil {
		t.Fatalf("LoadAssets failed: %v", err)
	}

	if cfg.Cleanup != false {
		t.Errorf("expected cleanup=false, got %v", cfg.Cleanup)
	}
	if len(cfg.Talos) != 2 {
		t.Fatalf("expected 2 talos assets, got %d", len(cfg.Talos))
	}
	if cfg.Talos[0].ID != "talos-v1.10.6" {
		t.Errorf("expected talos[0] id 'talos-v1.10.6', got %q", cfg.Talos[0].ID)
	}
	if cfg.Talos[0].Version != "v1.10.6" {
		t.Errorf("expected version 'v1.10.6', got %q", cfg.Talos[0].Version)
	}
	if cfg.Talos[1].ImageFactoryHash != "37f5a3fbd1e1e5d2a3c4" {
		t.Errorf("expected image factory hash, got %q", cfg.Talos[1].ImageFactoryHash)
	}
	if !cfg.Talos[1].DownloadUKI {
		t.Error("expected download_uki=true")
	}
	if len(cfg.Ubuntu) != 1 {
		t.Fatalf("expected 1 ubuntu asset, got %d", len(cfg.Ubuntu))
	}
}

func TestLoadAssets_Empty(t *testing.T) {
	path := writeTempFile(t, "assets.yaml", "")
	cfg, err := LoadAssets(path)
	if err != nil {
		t.Fatalf("LoadAssets on empty file should succeed: %v", err)
	}
	if cfg.Cleanup {
		t.Error("expected default cleanup=false")
	}
}

func TestAssetsConfig_FindAsset(t *testing.T) {
	cfg := &AssetsConfig{
		Talos: []TalosAsset{
			{ID: "talos-v1.10.6", Arch: "amd64"},
		},
		Ubuntu: []UbuntuAsset{
			{ID: "ubuntu-noble", Arch: "amd64"},
		},
	}

	tests := []struct {
		id       string
		wantType OSType
		found    bool
	}{
		{"talos-v1.10.6", OSTypeTalos, true},
		{"ubuntu-noble", OSTypeUbuntu, true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			info := cfg.FindAsset(tt.id)
			if !tt.found {
				if info != nil {
					t.Errorf("expected nil for %q, got %+v", tt.id, info)
				}
				return
			}
			if info == nil {
				t.Fatalf("expected to find asset %q, got nil", tt.id)
			}
			if info.OSType != tt.wantType {
				t.Errorf("expected OS type %q, got %q", tt.wantType, info.OSType)
			}
		})
	}
}

func TestAssetsConfig_AllAssetIDs(t *testing.T) {
	cfg := &AssetsConfig{
		Talos:  []TalosAsset{{ID: "talos-a"}},
		Ubuntu: []UbuntuAsset{{ID: "ubuntu-b"}},
	}

	ids := cfg.AllAssetIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
}
