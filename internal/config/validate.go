package config

import (
	"fmt"
	"strings"
)

// ValidationError is a collection of validation failures.
// It accumulates all errors found during validation so the operator
// sees every issue at once, not one at a time.
type ValidationError struct {
	Errors []string
}

func (ve *ValidationError) Error() string {
	if len(ve.Errors) == 1 {
		return ve.Errors[0]
	}
	return fmt.Sprintf("%d validation errors:\n  - %s", len(ve.Errors), strings.Join(ve.Errors, "\n  - "))
}

func (ve *ValidationError) add(format string, args ...any) {
	ve.Errors = append(ve.Errors, fmt.Sprintf(format, args...))
}

func (ve *ValidationError) hasErrors() bool {
	return len(ve.Errors) > 0
}

// FullConfig holds all three config files together for cross-reference validation.
type FullConfig struct {
	Machines *MachinesConfig
	Assets   *AssetsConfig
	Menu     *MenuConfig
}

// Validate performs full validation across all three config files:
//   - Individual field validation (required fields, formats)
//   - Duplicate MAC detection
//   - Cross-reference checking (profiles exist in assets, menu entries exist)
//
// Returns a ValidationError containing ALL problems found, so operators
// can fix everything in one pass.
func Validate(fc *FullConfig) error {
	ve := &ValidationError{}

	if fc.Machines != nil {
		validateMachines(ve, fc.Machines)
	}
	if fc.Assets != nil {
		validateAssets(ve, fc.Assets)
	}
	if fc.Menu != nil {
		validateMenu(ve, fc.Menu)
	}

	// Cross-reference validation only when we have the configs to cross-reference
	if fc.Machines != nil && fc.Assets != nil {
		validateMachineAssets(ve, fc.Machines, fc.Assets)
	}
	if fc.Menu != nil && fc.Assets != nil {
		validateMenuAssets(ve, fc.Menu, fc.Assets)
	}

	if ve.hasErrors() {
		return ve
	}
	return nil
}

func validateMachines(ve *ValidationError, cfg *MachinesConfig) {
	seenMACs := make(map[MAC]string) // MAC → hostname/source for duplicate detection

	for i, group := range cfg.Groups {
		if group.Name == "" {
			ve.add("groups[%d]: name is required", i)
		}
		if group.Profile == "" {
			ve.add("groups[%d] (%s): profile is required", i, group.Name)
		}
		if group.Template == "" {
			ve.add("groups[%d] (%s): template is required", i, group.Name)
		}
		if len(group.Machines) == 0 {
			ve.add("groups[%d] (%s): machines list is empty", i, group.Name)
		}

		for j, m := range group.Machines {
			if m.MAC == "" {
				ve.add("groups[%d] (%s).machines[%d]: mac is required", i, group.Name, j)
			}
			if m.Hostname == "" {
				ve.add("groups[%d] (%s).machines[%d]: hostname is required", i, group.Name, j)
			} else if !isValidHostname(m.Hostname) {
				ve.add("groups[%d] (%s).machines[%d]: hostname %q is not DNS-safe (lowercase alphanumeric and hyphens only)",
					i, group.Name, j, m.Hostname)
			}

			if m.MAC != "" {
				key := m.MAC
				if existing, ok := seenMACs[key]; ok {
					ve.add("duplicate MAC %s: defined in %s and %s", m.MAC, existing, m.Hostname)
				} else {
					seenMACs[key] = m.Hostname
				}
			}
		}
	}

	for i, s := range cfg.Singletons {
		if s.MAC == "" {
			ve.add("singletons[%d]: mac is required", i)
		}
		if s.Hostname == "" {
			ve.add("singletons[%d]: hostname is required", i)
		} else if !isValidHostname(s.Hostname) {
			ve.add("singletons[%d]: hostname %q is not DNS-safe", i, s.Hostname)
		}
		if s.Profile == "" {
			ve.add("singletons[%d] (%s): profile is required", i, s.Hostname)
		}
		if s.Config == "" {
			ve.add("singletons[%d] (%s): config is required", i, s.Hostname)
		}

		if s.MAC != "" {
			if existing, ok := seenMACs[s.MAC]; ok {
				ve.add("duplicate MAC %s: defined in %s and %s (singleton)", s.MAC, existing, s.Hostname)
			} else {
				seenMACs[s.MAC] = s.Hostname
			}
		}
	}
}

func validateAssets(ve *ValidationError, cfg *AssetsConfig) {
	seenIDs := make(map[string]bool)

	for i, a := range cfg.Talos {
		if a.ID == "" {
			ve.add("talos[%d]: id is required", i)
		} else if !isValidAssetID(a.ID) {
			ve.add("talos[%d]: id %q must be lowercase alphanumeric with hyphens", i, a.ID)
		}

		if a.Version == "" && a.ImageFactoryHash == "" {
			ve.add("talos[%d] (%s): version is required (image_factory_hash is optional)", i, a.ID)
		}
		if a.Version == "" && a.ImageFactoryHash != "" {
			ve.add("talos[%d] (%s): image_factory_hash requires version to be set (which Talos version to build)", i, a.ID)
		}
		if a.Arch == "" {
			ve.add("talos[%d] (%s): arch is required", i, a.ID)
		}

		if a.ID != "" {
			if seenIDs[a.ID] {
				ve.add("duplicate asset id %q", a.ID)
			}
			seenIDs[a.ID] = true
		}
	}

	for i, a := range cfg.Ubuntu {
		if a.ID == "" {
			ve.add("ubuntu[%d]: id is required", i)
		} else if !isValidAssetID(a.ID) {
			ve.add("ubuntu[%d]: id %q must be lowercase alphanumeric with hyphens", i, a.ID)
		}
		if a.Release == "" {
			ve.add("ubuntu[%d] (%s): release is required", i, a.ID)
		}
		if a.Arch == "" {
			ve.add("ubuntu[%d] (%s): arch is required", i, a.ID)
		}
		if a.ID != "" {
			if seenIDs[a.ID] {
				ve.add("duplicate asset id %q", a.ID)
			}
			seenIDs[a.ID] = true
		}
	}

	for i, a := range cfg.Debian {
		if a.ID == "" {
			ve.add("debian[%d]: id is required", i)
		} else if !isValidAssetID(a.ID) {
			ve.add("debian[%d]: id %q must be lowercase alphanumeric with hyphens", i, a.ID)
		}
		if a.Release == "" {
			ve.add("debian[%d] (%s): release is required", i, a.ID)
		}
		if a.Arch == "" {
			ve.add("debian[%d] (%s): arch is required", i, a.ID)
		}
		if a.ID != "" {
			if seenIDs[a.ID] {
				ve.add("duplicate asset id %q", a.ID)
			}
			seenIDs[a.ID] = true
		}
	}

	for i, a := range cfg.Arch {
		if a.ID == "" {
			ve.add("arch[%d]: id is required", i)
		} else if !isValidAssetID(a.ID) {
			ve.add("arch[%d]: id %q must be lowercase alphanumeric with hyphens", i, a.ID)
		}
		if a.Arch == "" {
			ve.add("arch[%d] (%s): arch is required", i, a.ID)
		}
		if a.ID != "" {
			if seenIDs[a.ID] {
				ve.add("duplicate asset id %q", a.ID)
			}
			seenIDs[a.ID] = true
		}
	}
}

func validateMenu(ve *ValidationError, cfg *MenuConfig) {
	if cfg.Timeout < 0 {
		ve.add("menu: timeout must be non-negative, got %d", cfg.Timeout)
	}
	if cfg.Default == "" {
		ve.add("menu: default is required")
	}
	if len(cfg.Entries) == 0 {
		ve.add("menu: at least one entry is required")
	}

	seenEntryIDs := make(map[string]bool)
	defaultFound := false

	for i, entry := range cfg.Entries {
		if entry.ID == "" {
			ve.add("menu.entries[%d]: id is required", i)
		}
		if entry.Label == "" {
			ve.add("menu.entries[%d] (%s): label is required", i, entry.ID)
		}
		if entry.Profile == "" {
			ve.add("menu.entries[%d] (%s): profile is required", i, entry.ID)
		}

		if entry.ID != "" {
			if seenEntryIDs[entry.ID] {
				ve.add("menu: duplicate entry id %q", entry.ID)
			}
			seenEntryIDs[entry.ID] = true
		}

		if entry.ID == cfg.Default {
			defaultFound = true
		}
	}

	if cfg.Default != "" && !defaultFound {
		ve.add("menu: default %q does not match any entry id", cfg.Default)
	}
}

func validateMachineAssets(ve *ValidationError, machines *MachinesConfig, assets *AssetsConfig) {
	for _, m := range machines.AllMachines() {
		if assets.FindAsset(m.Profile) == nil {
			ve.add("machine %s: profile %q not found in assets", m.Hostname, m.Profile)
		}
	}
}

func validateMenuAssets(ve *ValidationError, menu *MenuConfig, assets *AssetsConfig) {
	for _, entry := range menu.Entries {
		if assets.FindAsset(entry.Profile) == nil {
			ve.add("menu entry %q: profile %q not found in assets", entry.ID, entry.Profile)
		}
	}
}
