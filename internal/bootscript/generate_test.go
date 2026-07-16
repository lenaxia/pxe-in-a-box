package bootscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/config"
)

func TestGenerate_ValidMenu(t *testing.T) {
	menu := &config.MenuConfig{
		Timeout: 10,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos Linux (Maintenance Mode)", Profile: "talos-v1.10.6"},
			{ID: "ubuntu", Label: "Ubuntu Server 24.04", Profile: "ubuntu-noble"},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
		Ubuntu: []config.UbuntuAsset{
			{ID: "ubuntu-noble", Release: "noble", Arch: "amd64"},
		},
	}

	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputPath := filepath.Join(t.TempDir(), "boot.ipxe")

	if err := Generate(menu, assets, endpoint, outputPath); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	script := string(data)

	// Verify chain attempt to matchbox
	if !strings.Contains(script, "chain http://192.168.2.103:8081/ipxe?mac=${mac:hexhyp}") {
		t.Error("script should contain chain to matchbox /ipxe endpoint")
	}

	// Verify || goto menu fallback
	if !strings.Contains(script, "|| goto menu") {
		t.Error("script should contain '|| goto menu' fallback")
	}

	// Verify menu section
	if !strings.Contains(script, ":menu") {
		t.Error("script should contain :menu label")
	}
	if !strings.Contains(script, "menu PXE Boot Menu") {
		t.Error("script should contain menu title")
	}

	// Verify menu items
	if !strings.Contains(script, "item talos Talos Linux (Maintenance Mode)") {
		t.Error("script should contain talos menu item")
	}
	if !strings.Contains(script, "item ubuntu Ubuntu Server 24.04") {
		t.Error("script should contain ubuntu menu item")
	}

	// Verify choose with timeout
	if !strings.Contains(script, "choose --default talos --timeout 10000") {
		t.Error("script should contain choose with 10s timeout (10000ms)")
	}

	// Verify Talos maintenance mode (talos.config=null)
	if !strings.Contains(script, ":talos") {
		t.Error("script should contain :talos label")
	}
	if !strings.Contains(script, "talos.config=null") {
		t.Error("talos menu entry should use talos.config=null for maintenance mode")
	}
	if !strings.Contains(script, "/assets/talos-v1.10.6/amd64/vmlinuz") {
		t.Error("script should reference talos vmlinuz")
	}

	// Verify Ubuntu entry
	if !strings.Contains(script, ":ubuntu") {
		t.Error("script should contain :ubuntu label")
	}
	if !strings.Contains(script, "/assets/ubuntu-noble/amd64/linux") {
		t.Error("script should reference ubuntu linux kernel")
	}
}

func TestGenerate_TimeoutConversion(t *testing.T) {
	menu := &config.MenuConfig{
		Timeout: 5,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos", Profile: "talos-v1.10.6"},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
	}

	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputPath := filepath.Join(t.TempDir(), "boot.ipxe")

	if err := Generate(menu, assets, endpoint, outputPath); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	if !strings.Contains(string(data), "--timeout 5000") {
		t.Error("5s timeout should convert to 5000ms in iPXE script")
	}
}

func TestGenerate_FileWritten(t *testing.T) {
	menu := &config.MenuConfig{
		Timeout: 10,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos", Profile: "talos-v1.10.6"},
		},
	}

	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
	}

	endpoint := Endpoint{Address: "192.168.2.103", Port: 8081}
	outputPath := filepath.Join(t.TempDir(), "subdir", "boot.ipxe")

	if err := Generate(menu, assets, endpoint, outputPath); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file should exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output file should not be empty")
	}
}
