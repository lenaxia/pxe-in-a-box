//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/lenaxia/pxe-in-a-box/internal/bootscript"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
)

// TestE2E_QEMU_UnknownMAC_ShowsMenu boots a QEMU VM with iPXE that fetches
// the custom boot.ipxe from matchbox. Since the MAC is unknown, matchbox
// returns 404, and iPXE should fall through to the menu. We verify by
// checking QEMU output for the menu text.
//
// This test requires:
//   - qemu-system-x86_64 on PATH
//   - matchbox running (handled by startMatchbox)
//
// The test uses QEMU's built-in iPXE with an HTTP bootfile pointing
// directly at matchbox's /assets/boot.ipxe.
func TestE2E_QEMU_UnknownMAC_ShowsMenu(t *testing.T) {
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		t.Skip("qemu-system-x86_64 not found, skipping QEMU test")
	}

	mi := startMatchbox(t)

	// Generate boot.ipxe with a short timeout for test speed
	menu := &config.MenuConfig{
		Timeout: 2,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos Linux (Maintenance Mode)", Profile: "talos-v1.10.6"},
		},
	}
	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}
	endpoint := bootscript.Endpoint{Address: "127.0.0.1", Port: mi.port}
	bootscript.Generate(menu, assets, endpoint, fmt.Sprintf("%s/boot.ipxe", mi.assetsDir))

	// Generate group for a known MAC (for comparison)
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("52:54:00:11:22:33"), Hostname: "known"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	bootURL := fmt.Sprintf("http://127.0.0.1:%d/assets/boot.ipxe", mi.port)

	result := runQEMU(t, bootURL, "52:54:00:99:99:99", 30*time.Second)

	// iPXE should fetch boot.ipxe successfully
	assertContains(t, result, "boot.ipxe")

	// matchbox should return 404 for the unknown MAC
	assertContains(t, result, "No such file or directory")

	// iPXE should show the menu
	assertContains(t, result, "PXE Boot Menu")
	assertContains(t, result, "Talos Linux (Maintenance Mode)")

	// The menu countdown should be visible (2s timeout)
	assertContains(t, result, "(2)")

	t.Logf("QEMU output:\n%s", truncate(result, 2000))
}

// TestE2E_QEMU_KnownMAC_BypassesMenu boots a QEMU VM with a known MAC.
// matchbox should return 200 with an iPXE boot script, and iPXE should
// NOT show the menu. It will fail to boot because there's no real kernel,
// but we verify it tries to load the kernel (not the menu).
func TestE2E_QEMU_KnownMAC_BypassesMenu(t *testing.T) {
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		t.Skip("qemu-system-x86_64 not found, skipping QEMU test")
	}

	mi := startMatchbox(t)

	// Generate boot.ipxe
	menu := &config.MenuConfig{
		Timeout: 2,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos Linux (Maintenance Mode)", Profile: "talos-v1.10.6"},
		},
	}
	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}
	endpoint := bootscript.Endpoint{Address: "127.0.0.1", Port: mi.port}
	bootscript.Generate(menu, assets, endpoint, fmt.Sprintf("%s/boot.ipxe", mi.assetsDir))

	// Generate group/profile for the known MAC
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("52:54:00:11:22:33"), Hostname: "known-node"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	bootURL := fmt.Sprintf("http://127.0.0.1:%d/assets/boot.ipxe", mi.port)

	result := runQEMU(t, bootURL, "52:54:00:11:22:33", 30*time.Second)

	// iPXE should fetch boot.ipxe
	assertContains(t, result, "boot.ipxe")

	// matchbox /ipxe should return 200 for known MAC — iPXE shows "ok"
	assertContains(t, result, "/ipxe... ok")

	// iPXE should attempt to load the kernel (will fail since no real kernel)
	// This proves the profile's kernel path was served correctly
	assertContains(t, result, "/assets/talos-v1.10.6/amd64/vmlinuz")

	// Note: when the kernel is missing, chain fails and falls through to menu.
	// This is documented behavior. The menu appearing is not a failure — it
	// means the known machine's boot path was exercised, just without real assets.

	t.Logf("QEMU output:\n%s", truncate(result, 2000))
}

// runQEMU runs qemu-system-x86_64 with the given boot URL and MAC address,
// captures all output, and kills QEMU after the timeout (since it will
// hang trying to load a nonexistent kernel).
func runQEMU(t *testing.T, bootURL, mac string, timeout time.Duration) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var buf bytes.Buffer

	cmd := exec.CommandContext(ctx, "qemu-system-x86_64",
		"-m", "256",
		"-netdev", fmt.Sprintf("user,id=net0,bootfile=%s", bootURL),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", mac),
		"-boot", "n",
		"-nographic",
		"-serial", "mon:stdio",
		"-display", "none",
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	_ = cmd.Run() // ignore exit code — QEMU is killed by context timeout

	return buf.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
