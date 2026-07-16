//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/bootscript"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/matchbox"
)

// TestE2E_KnownMAC_GetsiPXEBootScript starts a real matchbox server with
// a generated group/profile, then verifies that a known MAC receives a
// proper iPXE boot script with the correct kernel, initrd, and talos.config URL.
func TestE2E_KnownMAC_GetsiPXEBootScript(t *testing.T) {
	mi := startMatchbox(t)

	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "workers",
				Profile:  "talos-v1.10.6",
				Template: "worker.yaml.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("52:54:00:12:34:56"), Hostname: "worker01"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// Create a fake kernel/initrd so matchbox's asset server has something
	mi.writeAsset("talos-v1.10.6/amd64/vmlinuz", "FAKE_KERNEL")
	mi.writeAsset("talos-v1.10.6/amd64/initramfs.xz", "FAKE_INITRAMFS")
	mi.writeAsset("rendered/worker01.yaml", "machine-config-content")

	status, body := mi.get("/ipxe?mac=52:54:00:12:34:56")

	if status != 200 {
		t.Fatalf("GET /ipxe for known MAC: status = %d, want 200", status)
	}

	// Response should be an iPXE script
	assertContains(t, body, "#!ipxe")
	assertContains(t, body, "kernel /assets/talos-v1.10.6/amd64/vmlinuz")
	assertContains(t, body, "initrd /assets/talos-v1.10.6/amd64/initramfs.xz")
	assertContains(t, body, "boot")
	assertContains(t, body, "talos.platform=metal")
	assertContains(t, body, "talos.config=http://127.0.0.1:")
	assertContains(t, body, "/assets/rendered/worker01.yaml")
}

// TestE2E_UnknownMAC_Gets404 verifies that an unknown MAC gets HTTP 404,
// which is critical for the chain || goto menu fallback pattern.
func TestE2E_UnknownMAC_Gets404(t *testing.T) {
	mi := startMatchbox(t)

	// Define one known machine
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name:     "g",
				Profile:  "talos-v1.10.6",
				Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("52:54:00:aa:bb:cc"), Hostname: "known"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// Known MAC → 200
	status, _ := mi.get("/ipxe?mac=52:54:00:aa:bb:cc")
	if status != 200 {
		t.Errorf("known MAC: status = %d, want 200", status)
	}

	// Unknown MAC → 404
	status, _ = mi.get("/ipxe?mac=52:54:00:99:99:99")
	if status != 404 {
		t.Errorf("unknown MAC: status = %d, want 404", status)
	}
}

// TestE2E_NoCatchAllGroup verifies that no catch-all group is generated,
// ensuring unknown MACs receive 404 (not 200 with a default profile).
func TestE2E_NoCatchAllGroup(t *testing.T) {
	mi := startMatchbox(t)

	// Generate groups for 3 machines
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:00:00:00:00:01"), Hostname: "m1"},
					{MAC: config.MAC("00:00:00:00:00:02"), Hostname: "m2"},
					{MAC: config.MAC("00:00:00:00:00:03"), Hostname: "m3"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// A MAC that's not in any group should get 404
	status, _ := mi.get("/ipxe?mac=00:00:00:ff:ff:ff")
	if status != 404 {
		t.Errorf("unmatched MAC should get 404, got %d", status)
	}
}

// TestE2E_MultipleMachines_IndividualProfiles verifies that each machine
// gets its own unique talos.config URL in the returned iPXE script.
func TestE2E_MultipleMachines_IndividualProfiles(t *testing.T) {
	mi := startMatchbox(t)

	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:00:00:00:00:a1"), Hostname: "worker01"},
					{MAC: config.MAC("00:00:00:00:00:a2"), Hostname: "worker02"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// worker01 should get its own config URL
	_, body1 := mi.get("/ipxe?mac=00:00:00:00:00:a1")
	assertContains(t, body1, "rendered/worker01.yaml")
	if strings.Contains(body1, "rendered/worker02.yaml") {
		t.Error("worker01 should NOT reference worker02.yaml")
	}

	// worker02 should get its own config URL
	_, body2 := mi.get("/ipxe?mac=00:00:00:00:00:a2")
	assertContains(t, body2, "rendered/worker02.yaml")
	if strings.Contains(body2, "rendered/worker01.yaml") {
		t.Error("worker02 should NOT reference worker01.yaml")
	}
}

// TestE2E_AssetsServed verifies that matchbox serves kernel/initramfs files
// via /assets/ path.
func TestE2E_AssetsServed(t *testing.T) {
	mi := startMatchbox(t)

	mi.writeAsset("talos-v1.10.6/amd64/vmlinuz", "KERNEL_BINARY_DATA")

	status, body := mi.get("/assets/talos-v1.10.6/amd64/vmlinuz")
	if status != 200 {
		t.Fatalf("GET /assets/.../vmlinuz: status = %d, want 200", status)
	}
	if body != "KERNEL_BINARY_DATA" {
		t.Errorf("body mismatch: got %q", body)
	}
}

// TestE2E_BootScript_ServedAndValid verifies that the generated boot.ipxe
// is served correctly and contains the chain || goto menu pattern.
func TestE2E_BootScript_ServedAndValid(t *testing.T) {
	mi := startMatchbox(t)

	// Generate and write boot.ipxe to assets
	menu := &config.MenuConfig{
		Timeout: 5,
		Default: "talos",
		Entries: []config.MenuEntry{
			{ID: "talos", Label: "Talos Maintenance", Profile: "talos-v1.10.6"},
		},
	}
	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}
	endpoint := bootscript.Endpoint{Address: "127.0.0.1", Port: mi.port}

	bootPath := filepath.Join(mi.assetsDir, "boot.ipxe")
	bootscript.Generate(menu, assets, endpoint, bootPath)

	status, body := mi.get("/assets/boot.ipxe")
	if status != 200 {
		t.Fatalf("GET /assets/boot.ipxe: status = %d, want 200", status)
	}

	assertContains(t, body, "chain http://127.0.0.1:")
	assertContains(t, body, "/ipxe?mac=${mac:hexhyp}")
	assertContains(t, body, "|| goto menu")
	assertContains(t, body, ":menu")
	assertContains(t, body, "item talos Talos Maintenance")
	assertContains(t, body, "choose --default talos --timeout 5000")
	assertContains(t, body, "talos.config=null")
}

// TestE2E_MACNormalization verifies that matchbox normalizes MAC addresses
// the same way we do (colon-separated lowercase). Both hexhyp and colon
// formats should resolve to the same group.
func TestE2E_MACNormalization(t *testing.T) {
	mi := startMatchbox(t)

	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("52:54:00:12:34:56"), Hostname: "test-node"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// matchbox's /boot.ipxe sends mac as ${mac:hexhyp} (dashes)
	// So the incoming query param is mac=52-54-00-12-34-56
	// matchbox normalizes this internally
	status, body := mi.get("/ipxe?mac=52-54-00-12-34-56")

	if status != 200 {
		t.Fatalf("GET /ipxe with hexhyp MAC: status = %d, want 200", status)
	}
	assertContains(t, body, "rendered/test-node.yaml")
}

// TestE2E_SingletonMachine verifies that singleton machines (with direct
// config files) get profiles pointing to /assets/static/ instead of /assets/rendered/.
func TestE2E_SingletonMachine(t *testing.T) {
	mi := startMatchbox(t)

	machines := &config.MachinesConfig{
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
		Talos: []config.TalosAsset{{ID: "talos-v1.9.1", Version: "v1.9.1", Arch: "amd64"}},
	}
	endpoint := matchbox.Endpoint{Address: "127.0.0.1", Port: mi.port}
	matchbox.GenerateGroups(machines, filepath.Join(mi.dataDir, "groups"))
	matchbox.GenerateProfiles(machines, assets, endpoint, filepath.Join(mi.dataDir, "profiles"))

	status, body := mi.get("/ipxe?mac=b8:ae:ed:73:c3:bc")
	if status != 200 {
		t.Fatalf("singleton MAC: status = %d, want 200", status)
	}

	// Should point to static/ not rendered/
	assertContains(t, body, "/assets/static/melfina.yaml")
	if strings.Contains(body, "/assets/rendered/") {
		t.Error("singleton should not reference rendered/")
	}
}

// TestE2E_GroupReloadWithoutRestart verifies that adding a new group JSON
// file is picked up by matchbox without restarting (matchbox reads from
// disk on each request).
func TestE2E_GroupReloadWithoutRestart(t *testing.T) {
	mi := startMatchbox(t)

	// Initially: no groups
	status, _ := mi.get("/ipxe?mac=00:00:00:00:00:01")
	if status != 404 {
		t.Fatalf("expected 404 with no groups, got %d", status)
	}

	// Add a group for this MAC (matchbox reads from disk each request)
	machines := &config.MachinesConfig{
		Groups: []config.Group{
			{
				Name: "g", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []config.GroupMachine{
					{MAC: config.MAC("00:00:00:00:00:01"), Hostname: "newnode"},
				},
			},
		},
	}
	mi.reloadGroups(machines)

	// Now it should return 200 — no restart needed
	status, body := mi.get("/ipxe?mac=00:00:00:00:00:01")
	if status != 200 {
		t.Fatalf("expected 200 after adding group, got %d", status)
	}
	assertContains(t, body, "rendered/newnode.yaml")
}
