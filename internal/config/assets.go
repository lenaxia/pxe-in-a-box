package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AssetsConfig is the top-level structure of assets.yaml.
// It defines all OS assets the system should download and manage.
type AssetsConfig struct {
	Cleanup bool          `yaml:"cleanup"`
	Talos   []TalosAsset  `yaml:"talos"`
	Ubuntu  []UbuntuAsset `yaml:"ubuntu"`
	Debian  []DebianAsset `yaml:"debian"`
	Arch    []ArchAsset   `yaml:"arch"`
}

// TalosAsset defines a Talos kernel/initramfs pair to download.
// Either Version (official release) or ImageFactoryHash (custom build)
// must be specified, but not both.
type TalosAsset struct {
	ID               string            `yaml:"id"`
	Version          string            `yaml:"version,omitempty"`
	Arch             string            `yaml:"arch"`
	ImageFactoryHash string            `yaml:"image_factory_hash,omitempty"`
	DownloadUKI      bool              `yaml:"download_uki,omitempty"`
	SHA256           map[string]string `yaml:"sha256,omitempty"`
}

// UbuntuAsset defines an Ubuntu netboot asset to download.
type UbuntuAsset struct {
	ID      string `yaml:"id"`
	Release string `yaml:"release"`
	Arch    string `yaml:"arch"`
}

// DebianAsset defines a Debian netboot asset to download.
type DebianAsset struct {
	ID      string `yaml:"id"`
	Release string `yaml:"release"`
	Arch    string `yaml:"arch"`
}

// ArchAsset defines an Arch Linux boot asset to download.
type ArchAsset struct {
	ID   string `yaml:"id"`
	Arch string `yaml:"arch"`
}

// OSType identifies which OS family an asset belongs to.
type OSType string

const (
	OSTypeTalos  OSType = "talos"
	OSTypeUbuntu OSType = "ubuntu"
	OSTypeDebian OSType = "debian"
	OSTypeArch   OSType = "arch"
)

// AssetInfo describes a resolved asset with enough context to generate
// boot profiles and download paths.
type AssetInfo struct {
	ID     string
	OSType OSType
	Arch   string
}

// FindAsset searches all OS sections for an asset with the given ID.
// Returns nil if not found.
func (c *AssetsConfig) FindAsset(id string) *AssetInfo {
	for _, a := range c.Talos {
		if a.ID == id {
			return &AssetInfo{ID: a.ID, OSType: OSTypeTalos, Arch: a.Arch}
		}
	}
	for _, a := range c.Ubuntu {
		if a.ID == id {
			return &AssetInfo{ID: a.ID, OSType: OSTypeUbuntu, Arch: a.Arch}
		}
	}
	for _, a := range c.Debian {
		if a.ID == id {
			return &AssetInfo{ID: a.ID, OSType: OSTypeDebian, Arch: a.Arch}
		}
	}
	for _, a := range c.Arch {
		if a.ID == id {
			return &AssetInfo{ID: a.ID, OSType: OSTypeArch, Arch: a.Arch}
		}
	}
	return nil
}

// AllAssetIDs returns the set of all asset IDs across all OS types.
func (c *AssetsConfig) AllAssetIDs() []string {
	var ids []string
	for _, a := range c.Talos {
		ids = append(ids, a.ID)
	}
	for _, a := range c.Ubuntu {
		ids = append(ids, a.ID)
	}
	for _, a := range c.Debian {
		ids = append(ids, a.ID)
	}
	for _, a := range c.Arch {
		ids = append(ids, a.ID)
	}
	return ids
}

// LoadAssets reads and parses an assets.yaml file.
func LoadAssets(path string) (*AssetsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading assets config %s: %w", path, err)
	}

	var cfg AssetsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing assets config %s: %w", path, err)
	}

	return &cfg, nil
}
