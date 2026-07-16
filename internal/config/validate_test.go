package config

import (
	"strings"
	"testing"
)

func validAssets() *AssetsConfig {
	return &AssetsConfig{
		Talos: []TalosAsset{
			{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"},
		},
		Ubuntu: []UbuntuAsset{
			{ID: "ubuntu-noble", Release: "noble", Arch: "amd64"},
		},
	}
}

func mustMACPanic(s string) MAC {
	m, err := NewMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func validMachines() *MachinesConfig {
	return &MachinesConfig{
		Groups: []Group{
			{
				Name:     "workers",
				Profile:  "talos-v1.10.6",
				Template: "worker.yaml.j2",
				Machines: []GroupMachine{
					{MAC: mustMACPanic("00:e0:4c:68:00:a1"), Hostname: "worker01"},
				},
			},
		},
	}
}

func validMenu() *MenuConfig {
	return &MenuConfig{
		Timeout: 10,
		Default: "talos-maintenance",
		Entries: []MenuEntry{
			{ID: "talos-maintenance", Label: "Talos", Profile: "talos-v1.10.6"},
			{ID: "ubuntu", Label: "Ubuntu", Profile: "ubuntu-noble"},
		},
	}
}

func TestValidate_AllValid(t *testing.T) {
	fc := &FullConfig{
		Machines: validMachines(),
		Assets:   validAssets(),
		Menu:     validMenu(),
	}
	if err := Validate(fc); err != nil {
		t.Fatalf("expected no validation errors, got: %v", err)
	}
}

func TestValidate_DuplicateMAC(t *testing.T) {
	cfg := &MachinesConfig{
		Groups: []Group{
			{
				Name:     "g1",
				Profile:  "talos-v1.10.6",
				Template: "t.j2",
				Machines: []GroupMachine{
					{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "m1"},
					{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "m2"},
				},
			},
		},
	}
	fc := &FullConfig{Machines: cfg}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected duplicate MAC error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate MAC") {
		t.Errorf("expected 'duplicate MAC' in error, got: %v", err)
	}
}

func TestValidate_DuplicateMACAcrossGroupsAndSingletons(t *testing.T) {
	cfg := &MachinesConfig{
		Groups: []Group{
			{
				Name:     "g1",
				Profile:  "talos-v1.10.6",
				Template: "t.j2",
				Machines: []GroupMachine{
					{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "m1"},
				},
			},
		},
		Singletons: []Singleton{
			{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "s1", Profile: "p", Config: "c.yaml"},
		},
	}
	fc := &FullConfig{Machines: cfg}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected duplicate MAC error across group and singleton")
	}
}

func TestValidate_MissingProfileInAssets(t *testing.T) {
	fc := &FullConfig{
		Machines: &MachinesConfig{
			Groups: []Group{
				{
					Name:     "g1",
					Profile:  "nonexistent-asset",
					Template: "t.j2",
					Machines: []GroupMachine{
						{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "m1"},
					},
				},
			},
		},
		Assets: validAssets(),
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for missing profile in assets")
	}
	if !strings.Contains(err.Error(), "not found in assets") {
		t.Errorf("expected 'not found in assets' in error, got: %v", err)
	}
}

func TestValidate_TalosVersionAndHashMutuallyExclusive(t *testing.T) {
	// Having both version + image_factory_hash is now VALID
	// (IF needs both: hash = schematic, version = Talos release)
	fc := &FullConfig{
		Assets: &AssetsConfig{
			Talos: []TalosAsset{
				{
					ID:               "valid-if",
					Version:          "v1.10.6",
					ImageFactoryHash: "abc123",
					Arch:             "amd64",
				},
			},
		},
	}
	err := Validate(fc)
	if err != nil {
		t.Fatalf("version + image_factory_hash should be valid together: %v", err)
	}
}

func TestValidate_TalosHashWithoutVersion(t *testing.T) {
	fc := &FullConfig{
		Assets: &AssetsConfig{
			Talos: []TalosAsset{
				{
					ID:               "broken",
					ImageFactoryHash: "abc123",
					Arch:             "amd64",
				},
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for image_factory_hash without version")
	}
}

func TestValidate_TalosMissingVersionAndHash(t *testing.T) {
	fc := &FullConfig{
		Assets: &AssetsConfig{
			Talos: []TalosAsset{
				{ID: "broken", Arch: "amd64"},
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for missing version and hash")
	}
}

func TestValidate_DuplicateAssetID(t *testing.T) {
	fc := &FullConfig{
		Assets: &AssetsConfig{
			Talos: []TalosAsset{
				{ID: "shared-id", Version: "v1.10.6", Arch: "amd64"},
			},
			Ubuntu: []UbuntuAsset{
				{ID: "shared-id", Release: "noble", Arch: "amd64"},
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for duplicate asset ID")
	}
	if !strings.Contains(err.Error(), "duplicate asset id") {
		t.Errorf("expected 'duplicate asset id' in error, got: %v", err)
	}
}

func TestValidate_InvalidHostname(t *testing.T) {
	fc := &FullConfig{
		Machines: &MachinesConfig{
			Groups: []Group{
				{
					Name:     "g1",
					Profile:  "talos-v1.10.6",
					Template: "t.j2",
					Machines: []GroupMachine{
						{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "UPPERCASE"},
					},
				},
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for invalid hostname")
	}
	if !strings.Contains(err.Error(), "not DNS-safe") {
		t.Errorf("expected 'not DNS-safe' in error, got: %v", err)
	}
}

func TestValidate_MenuDefaultNotFound(t *testing.T) {
	fc := &FullConfig{
		Menu: &MenuConfig{
			Timeout: 5,
			Default: "nonexistent",
			Entries: []MenuEntry{
				{ID: "talos", Label: "Talos", Profile: "talos-v1.10.6"},
			},
		},
		Assets: validAssets(),
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for default not matching entries")
	}
}

func TestValidate_MenuEntryProfileNotInAssets(t *testing.T) {
	fc := &FullConfig{
		Menu: &MenuConfig{
			Timeout: 5,
			Default: "talos",
			Entries: []MenuEntry{
				{ID: "talos", Label: "Talos", Profile: "nonexistent"},
			},
		},
		Assets: validAssets(),
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for menu entry profile not in assets")
	}
	if !strings.Contains(err.Error(), "not found in assets") {
		t.Errorf("expected 'not found in assets' in error, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	fc := &FullConfig{
		Machines: &MachinesConfig{
			Groups: []Group{
				{
					Name:     "", // missing name
					Profile:  "", // missing profile
					Template: "", // missing template
					Machines: []GroupMachine{
						{MAC: mustMAC(t, "00:e0:4c:68:00:a1"), Hostname: "BAD_NAME"}, // invalid hostname
					},
				},
			},
		},
		Assets: &AssetsConfig{
			Talos: []TalosAsset{
				{ID: "bad id with spaces", Version: "v1.10.6", Arch: "amd64"}, // invalid ID
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected multiple validation errors")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(ve.Errors), ve.Errors)
	}
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	fc := &FullConfig{
		Machines: &MachinesConfig{
			Singletons: []Singleton{
				{}, // all fields empty
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for empty singleton")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	// Should report mac, hostname, profile, config all missing
	found := map[string]bool{}
	for _, e := range ve.Errors {
		if strings.Contains(e, "mac is required") {
			found["mac"] = true
		}
		if strings.Contains(e, "hostname is required") {
			found["hostname"] = true
		}
		if strings.Contains(e, "profile is required") {
			found["profile"] = true
		}
		if strings.Contains(e, "config is required") {
			found["config"] = true
		}
	}
	for key := range found {
		if !found[key] {
			t.Errorf("expected error about missing %s", key)
		}
	}
}
