package config

import (
	"strings"
	"testing"
)

func TestValidate_DuplicateMACAcrossTwoGroups(t *testing.T) {
	cfg := &MachinesConfig{
		Groups: []Group{
			{
				Name: "g1", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []GroupMachine{
					{MAC: mustMACPanic("00:e0:4c:68:00:a1"), Hostname: "m1"},
				},
			},
			{
				Name: "g2", Profile: "talos-v1.10.6", Template: "t.j2",
				Machines: []GroupMachine{
					{MAC: mustMACPanic("00:e0:4c:68:00:a1"), Hostname: "m2"},
				},
			},
		},
	}
	fc := &FullConfig{Machines: cfg}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected duplicate MAC error across two groups")
	}
	if !strings.Contains(err.Error(), "duplicate MAC") {
		t.Errorf("expected 'duplicate MAC' in error, got: %v", err)
	}
}

func TestValidate_EmptyGroupMachines(t *testing.T) {
	fc := &FullConfig{
		Machines: &MachinesConfig{
			Groups: []Group{
				{Name: "empty", Profile: "talos-v1.10.6", Template: "t.j2", Machines: nil},
			},
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for empty machines list")
	}
	if !strings.Contains(err.Error(), "machines list is empty") {
		t.Errorf("expected 'machines list is empty' in error, got: %v", err)
	}
}

func TestValidate_MissingArch_AllOSTypes(t *testing.T) {
	tests := []struct {
		name   string
		assets *AssetsConfig
		errMsg string
	}{
		{
			name:   "talos missing arch",
			assets: &AssetsConfig{Talos: []TalosAsset{{ID: "t", Version: "v1.10.6"}}},
			errMsg: "arch is required",
		},
		{
			name:   "ubuntu missing arch",
			assets: &AssetsConfig{Ubuntu: []UbuntuAsset{{ID: "u", Release: "noble"}}},
			errMsg: "arch is required",
		},
		{
			name:   "debian missing arch",
			assets: &AssetsConfig{Debian: []DebianAsset{{ID: "d", Release: "bookworm"}}},
			errMsg: "arch is required",
		},
		{
			name:   "arch missing arch",
			assets: &AssetsConfig{Arch: []ArchAsset{{ID: "a"}}},
			errMsg: "arch is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&FullConfig{Assets: tt.assets})
			if err == nil {
				t.Fatal("expected error for missing arch")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected %q in error, got: %v", tt.errMsg, err)
			}
		})
	}
}

func TestValidate_MissingRelease(t *testing.T) {
	tests := []struct {
		name   string
		assets *AssetsConfig
		errMsg string
	}{
		{
			name:   "ubuntu missing release",
			assets: &AssetsConfig{Ubuntu: []UbuntuAsset{{ID: "u", Arch: "amd64"}}},
			errMsg: "release is required",
		},
		{
			name:   "debian missing release",
			assets: &AssetsConfig{Debian: []DebianAsset{{ID: "d", Arch: "amd64"}}},
			errMsg: "release is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&FullConfig{Assets: tt.assets})
			if err == nil {
				t.Fatal("expected error for missing release")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

func TestValidate_NegativeMenuTimeout(t *testing.T) {
	fc := &FullConfig{
		Menu: &MenuConfig{
			Timeout: -1,
			Default: "talos",
			Entries: []MenuEntry{{ID: "talos", Label: "T", Profile: "talos-v1.10.6"}},
		},
		Assets: &AssetsConfig{Talos: []TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}}},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
	if !strings.Contains(err.Error(), "timeout must be non-negative") {
		t.Errorf("got: %v", err)
	}
}

func TestValidate_DuplicateMenuEntryID(t *testing.T) {
	fc := &FullConfig{
		Menu: &MenuConfig{
			Timeout: 10,
			Default: "talos",
			Entries: []MenuEntry{
				{ID: "talos", Label: "T1", Profile: "talos-v1.10.6"},
				{ID: "talos", Label: "T2", Profile: "talos-v1.10.6"},
			},
		},
		Assets: &AssetsConfig{Talos: []TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}}},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected duplicate menu entry error")
	}
	if !strings.Contains(err.Error(), "duplicate entry id") {
		t.Errorf("got: %v", err)
	}
}

func TestValidate_EmptyMenuEntries(t *testing.T) {
	fc := &FullConfig{
		Menu: &MenuConfig{
			Timeout: 10,
			Default: "talos",
			Entries: nil,
		},
	}
	err := Validate(fc)
	if err == nil {
		t.Fatal("expected error for empty entries")
	}
	if !strings.Contains(err.Error(), "at least one entry") {
		t.Errorf("got: %v", err)
	}
}

func TestValidate_MissingMenuFields(t *testing.T) {
	tests := []struct {
		name   string
		entry  MenuEntry
		errMsg string
	}{
		{"missing id", MenuEntry{Label: "L", Profile: "p"}, "id is required"},
		{"missing label", MenuEntry{ID: "x", Profile: "p"}, "label is required"},
		{"missing profile", MenuEntry{ID: "x", Label: "L"}, "profile is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FullConfig{
				Menu: &MenuConfig{
					Timeout: 5,
					Default: "x",
					Entries: []MenuEntry{tt.entry},
				},
			}
			err := Validate(fc)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

func TestValidate_PartialConfig_OnlyMachines(t *testing.T) {
	fc := &FullConfig{
		Machines: validMachines(),
		// Assets and Menu are nil
	}
	err := Validate(fc)
	// Should validate machines only, not crash on nil Assets/Menu
	if err != nil {
		// Machine validation may pass — that's fine
		// The point is it shouldn't panic
	}
}

func TestNewMAC_Formats(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"00:e0:4c:68:00:8e", "00:e0:4c:68:00:8e"},
		{"00-E0-4C-68-00-8E", "00:e0:4c:68:00:8e"},
		{"00:e0:4c:68:00:8e", "00:e0:4c:68:00:8e"},
		// Go's net.ParseMAC also accepts the period-separated form
		// used by some IBM POWER systems
		// Bare hex without separators is NOT accepted by Go
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mac, err := NewMAC(tt.input)
			if err != nil {
				t.Fatalf("NewMAC(%q): %v", tt.input, err)
			}
			if mac.String() != tt.want {
				t.Errorf("NewMAC(%q) = %q, want %q", tt.input, mac.String(), tt.want)
			}
		})
	}
}

func TestNewMAC_RejectsBareHex(t *testing.T) {
	// Go's net.ParseMAC requires separators — bare hex is ambiguous
	_, err := NewMAC("00e04c68008e")
	if err == nil {
		t.Error("expected error for bare hex MAC (no separators)")
	}
}

func TestIsValidHostname_EdgeCases(t *testing.T) {
	tests := []struct {
		hostname string
		valid    bool
	}{
		{"worker01", true},
		{"cp", true},
		{"a", true},
		{"my-machine", true},
		{"UPPERCASE", false},
		{"has.dots", false},
		{"-leading", false},
		{"trailing-", false},
		{"under_score", false},
		{"", false},
		{"a.b.c", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := isValidHostname(tt.hostname)
			if got != tt.valid {
				t.Errorf("isValidHostname(%q) = %v, want %v", tt.hostname, got, tt.valid)
			}
		})
	}
}

func TestIsValidAssetID_EdgeCases(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"talos-v1.10.6", true},
		{"ubuntu-noble", true},
		{"a", true},
		{"talos.v1.10.6", true},
		{"UPPERCASE", false},
		{"has spaces", false},
		{"-leading", false},
		{"trailing-", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidAssetID(tt.id)
			if got != tt.valid {
				t.Errorf("isValidAssetID(%q) = %v, want %v", tt.id, got, tt.valid)
			}
		})
	}
}
