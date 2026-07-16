package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MachinesConfig is the top-level structure of machines.yaml.
// It defines all machines that should be PXE-booted with known profiles.
type MachinesConfig struct {
	Groups     []Group     `yaml:"groups"`
	Singletons []Singleton `yaml:"singletons"`
}

// Group is a collection of machines that share the same boot profile,
// machine config template, and default variables.
type Group struct {
	Name     string            `yaml:"name"`
	Profile  string            `yaml:"profile"`
	Template string            `yaml:"template"`
	Vars     map[string]string `yaml:"vars,omitempty"`
	Machines []GroupMachine    `yaml:"machines"`
}

// GroupMachine is a single machine within a group.
// Vars are merged with (and override) the group's Vars for template rendering.
type GroupMachine struct {
	MAC      MAC               `yaml:"mac"`
	Hostname string            `yaml:"hostname"`
	Vars     map[string]string `yaml:"vars,omitempty"`
}

// Singleton is a standalone machine that uses a direct config file
// instead of a template. Used for one-off machines that don't fit a group.
type Singleton struct {
	MAC      MAC    `yaml:"mac"`
	Hostname string `yaml:"hostname"`
	Profile  string `yaml:"profile"`
	Config   string `yaml:"config"`
}

// AllMachines returns every machine defined in groups and singletons.
// This is used for duplicate MAC detection and iteration during generation.
type DefinedMachine struct {
	MAC      MAC
	Hostname string
	Profile  string
	// Template is empty for singletons (they use a direct config file).
	Template string
	// Config is the direct config filename for singletons. Empty for group machines.
	Config string
	// Vars are the merged group + machine variables. Empty for singletons.
	Vars map[string]string
	// Source identifies where this machine was defined (group name or "singleton").
	Source string
}

// AllMachines returns every machine across all groups and singletons.
func (c *MachinesConfig) AllMachines() []DefinedMachine {
	var machines []DefinedMachine

	for _, group := range c.Groups {
		for _, m := range group.Machines {
			merged := mergeVars(group.Vars, m.Vars)
			machines = append(machines, DefinedMachine{
				MAC:      m.MAC,
				Hostname: m.Hostname,
				Profile:  group.Profile,
				Template: group.Template,
				Vars:     merged,
				Source:   group.Name,
			})
		}
	}

	for _, s := range c.Singletons {
		machines = append(machines, DefinedMachine{
			MAC:      s.MAC,
			Hostname: s.Hostname,
			Profile:  s.Profile,
			Config:   s.Config,
			Source:   "singleton",
		})
	}

	return machines
}

// LoadMachines reads and parses a machines.yaml file.
func LoadMachines(path string) (*MachinesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading machines config %s: %w", path, err)
	}

	var cfg MachinesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing machines config %s: %w", path, err)
	}

	return &cfg, nil
}

// mergeVars merges group vars with machine-specific vars.
// Machine vars take precedence on key conflicts.
func mergeVars(groupVars, machineVars map[string]string) map[string]string {
	if len(groupVars) == 0 && len(machineVars) == 0 {
		return nil
	}

	merged := make(map[string]string, len(groupVars)+len(machineVars))
	for k, v := range groupVars {
		merged[k] = v
	}
	for k, v := range machineVars {
		merged[k] = v
	}
	return merged
}
