package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MenuConfig is the top-level structure of menu.yaml.
// It defines the iPXE boot menu shown to unknown machines.
type MenuConfig struct {
	Timeout int         `yaml:"timeout"`
	Default string      `yaml:"default"`
	Entries []MenuEntry `yaml:"entries"`
}

// MenuEntry is a single option in the boot menu.
type MenuEntry struct {
	ID      string `yaml:"id"`
	Label   string `yaml:"label"`
	Profile string `yaml:"profile"`
}

// LoadMenu reads and parses a menu.yaml file.
func LoadMenu(path string) (*MenuConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading menu config %s: %w", path, err)
	}

	var cfg MenuConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing menu config %s: %w", path, err)
	}

	return &cfg, nil
}
