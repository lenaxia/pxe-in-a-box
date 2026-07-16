// Package matchbox generates matchbox-native group and profile JSON files
// from the higher-level PXE-in-a-Box config structures.
//
// Matchbox groups map MAC selectors to profiles.
// Matchbox profiles define boot config (kernel, initrd, kernel args).
//
// We generate one group + one profile per machine, because each machine
// needs a unique talos.config URL in its kernel args, and matchbox does
// not support templating in kernel args.
package matchbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lenaxia/pxe-in-a-box/internal/config"
)

// GroupFile is the matchbox-native group JSON structure.
// Matchbox reads these from the groups/ directory.
type GroupFile struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Profile  string            `json:"profile"`
	Selector map[string]string `json:"selector,omitempty"`
}

// ProfileFile is the matchbox-native profile JSON structure.
// Matchbox reads these from the profiles/ directory.
type ProfileFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Boot Boot   `json:"boot"`
}

// Boot defines the network boot settings for a profile.
type Boot struct {
	Kernel string   `json:"kernel"`
	Initrd []string `json:"initrd"`
	Args   []string `json:"args"`
}

// Endpoint describes the matchbox HTTP server address.
// Used to construct URLs in kernel args (e.g., talos.config).
type Endpoint struct {
	Address string // e.g., "192.168.2.103"
	Port    int    // e.g., 8081
}

// String returns the base URL (e.g., "http://192.168.2.103:8081").
func (e Endpoint) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", e.Address, e.Port)
}

// GenerateGroups writes one group JSON per machine to the output directory.
// The directory is cleared before writing to remove stale groups from
// removed machines.
func GenerateGroups(machines *config.MachinesConfig, outputDir string) error {
	if err := clearDir(outputDir); err != nil {
		return fmt.Errorf("clearing groups output dir: %w", err)
	}

	for _, m := range machines.AllMachines() {
		group := GroupFile{
			ID:      m.Hostname,
			Name:    m.Hostname,
			Profile: ProfileID(m.Profile, m.Hostname),
			Selector: map[string]string{
				"mac": m.MAC.String(),
			},
		}

		path := filepath.Join(outputDir, m.Hostname+".json")
		if err := writeJSON(path, group); err != nil {
			return fmt.Errorf("writing group for %s: %w", m.Hostname, err)
		}
	}

	return nil
}

// GenerateProfiles writes one profile JSON per machine to the output directory.
// Each profile contains the per-machine talos.config URL.
// The directory is cleared before writing to remove stale profiles.
func GenerateProfiles(machines *config.MachinesConfig, assets *config.AssetsConfig, endpoint Endpoint, outputDir string) error {
	if err := clearDir(outputDir); err != nil {
		return fmt.Errorf("clearing profiles output dir: %w", err)
	}

	for _, m := range machines.AllMachines() {
		profile, err := buildProfile(m, assets, endpoint)
		if err != nil {
			return fmt.Errorf("building profile for %s: %w", m.Hostname, err)
		}

		filename := fmt.Sprintf("%s.json", profile.ID)
		path := filepath.Join(outputDir, filename)
		if err := writeJSON(path, profile); err != nil {
			return fmt.Errorf("writing profile for %s: %w", m.Hostname, err)
		}
	}

	return nil
}

// ProfileID constructs the profile ID from asset ID and hostname.
// Convention: "<asset-id>-<hostname>" (e.g., "talos-v1.10.6-worker01").
func ProfileID(assetID, hostname string) string {
	return fmt.Sprintf("%s-%s", assetID, hostname)
}

// buildProfile creates a profile for a single machine.
func buildProfile(m config.DefinedMachine, assets *config.AssetsConfig, endpoint Endpoint) (*ProfileFile, error) {
	info := assets.FindAsset(m.Profile)
	if info == nil {
		return nil, fmt.Errorf("asset %q not found in assets config", m.Profile)
	}

	kernelPath, initrdPath, err := assetBootPaths(info)
	if err != nil {
		return nil, err
	}

	// Determine the machine config URL for talos.config
	var configURL string
	if m.Config != "" {
		// Singleton: points to a static config file
		configURL = fmt.Sprintf("%s/assets/static/%s", endpoint.BaseURL(), m.Config)
	} else {
		// Group machine: points to rendered config
		configURL = fmt.Sprintf("%s/assets/rendered/%s.yaml", endpoint.BaseURL(), m.Hostname)
	}

	// Base kernel args — standard for all Talos metal boots
	args := []string{
		"initrd=initramfs.xz",
		"init_on_alloc=1",
		"slab_nomerge",
		"pti=on",
		"console=tty0",
		"printk.devkmsg=on",
		"talos.platform=metal",
		fmt.Sprintf("talos.config=%s", configURL),
	}

	profile := &ProfileFile{
		ID:   ProfileID(m.Profile, m.Hostname),
		Name: fmt.Sprintf("%s - %s", m.Profile, m.Hostname),
		Boot: Boot{
			Kernel: kernelPath,
			Initrd: []string{initrdPath},
			Args:   args,
		},
	}

	return profile, nil
}

// assetBootPaths returns the kernel and initrd asset paths for a given
// asset type. The paths are relative to matchbox's assets root (e.g., "/assets/...").
func assetBootPaths(info *config.AssetInfo) (kernel, initrd string, err error) {
	base := fmt.Sprintf("/assets/%s/%s", info.ID, info.Arch)

	switch info.OSType {
	case config.OSTypeTalos:
		return fmt.Sprintf("%s/vmlinuz", base), fmt.Sprintf("%s/initramfs.xz", base), nil
	case config.OSTypeUbuntu:
		return fmt.Sprintf("%s/linux", base), fmt.Sprintf("%s/initrd", base), nil
	case config.OSTypeDebian:
		return fmt.Sprintf("%s/linux", base), fmt.Sprintf("%s/initrd.gz", base), nil
	case config.OSTypeArch:
		return fmt.Sprintf("%s/vmlinuz-linux", base), fmt.Sprintf("%s/initramfs-linux.img", base), nil
	default:
		return "", "", fmt.Errorf("unknown OS type %q for asset %q", info.OSType, info.ID)
	}
}

// writeJSON marshals a value to indented JSON and writes it to a file.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// clearDir removes all files from a directory, then recreates it.
// This ensures stale generated files from removed machines don't persist.
func clearDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}
