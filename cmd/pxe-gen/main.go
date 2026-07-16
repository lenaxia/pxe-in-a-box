// Command pxe-gen reads machine, asset, and menu configs, validates them,
// and generates matchbox groups, profiles, and the boot.ipxe script.
//
// This command runs on the Ansible controller during deployment.
// Ansible calls it before copying generated files to the target host.
//
// Usage:
//
//	pxe-gen --machines machines.yaml --assets assets.yaml --menu menu.yaml \
//	  --addr 192.168.2.103 --port 8081 --output-dir ./generated
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/homelab/pxe-in-a-box/internal/bootscript"
	"github.com/homelab/pxe-in-a-box/internal/config"
	"github.com/homelab/pxe-in-a-box/internal/matchbox"
)

func main() {
	var (
		machinesPath string
		assetsPath   string
		menuPath     string
		addr         string
		port         int
		outputDir    string
	)

	flag.StringVar(&machinesPath, "machines", "", "path to machines.yaml (required)")
	flag.StringVar(&assetsPath, "assets", "", "path to assets.yaml (required)")
	flag.StringVar(&menuPath, "menu", "", "path to menu.yaml (required)")
	flag.StringVar(&addr, "addr", "", "matchbox HTTP address, e.g. 192.168.2.103 (required)")
	flag.IntVar(&port, "port", 8081, "matchbox HTTP port")
	flag.StringVar(&outputDir, "output-dir", "./generated", "output directory for generated files")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pxe-gen generates matchbox groups, profiles, and boot.ipxe from config files.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: pxe-gen [flags]\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if machinesPath == "" || assetsPath == "" || menuPath == "" || addr == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := run(machinesPath, assetsPath, menuPath, addr, port, outputDir); err != nil {
		log.Fatalf("pxe-gen: %v", err)
	}
}

func run(machinesPath, assetsPath, menuPath, addr string, port int, outputDir string) error {
	machines, err := config.LoadMachines(machinesPath)
	if err != nil {
		return fmt.Errorf("loading machines: %w", err)
	}

	assets, err := config.LoadAssets(assetsPath)
	if err != nil {
		return fmt.Errorf("loading assets: %w", err)
	}

	menu, err := config.LoadMenu(menuPath)
	if err != nil {
		return fmt.Errorf("loading menu: %w", err)
	}

	fc := &config.FullConfig{Machines: machines, Assets: assets, Menu: menu}
	if err := config.Validate(fc); err != nil {
		return fmt.Errorf("validation:\n%w", err)
	}

	endpoint := matchbox.Endpoint{Address: addr, Port: port}

	groupsDir := filepath.Join(outputDir, "groups")
	profilesDir := filepath.Join(outputDir, "profiles")

	if err := matchbox.GenerateGroups(machines, groupsDir); err != nil {
		return fmt.Errorf("generating groups: %w", err)
	}
	log.Printf("generated %d group files in %s", len(machines.AllMachines()), groupsDir)

	if err := matchbox.GenerateProfiles(machines, assets, endpoint, profilesDir); err != nil {
		return fmt.Errorf("generating profiles: %w", err)
	}
	log.Printf("generated %d profile files in %s", len(machines.AllMachines()), profilesDir)

	bootPath := filepath.Join(outputDir, "assets", "boot.ipxe")
	if err := bootscript.Generate(menu, assets, bootscript.Endpoint(endpoint), bootPath); err != nil {
		return fmt.Errorf("generating boot.ipxe: %w", err)
	}
	log.Printf("generated boot.ipxe at %s", bootPath)

	return nil
}
