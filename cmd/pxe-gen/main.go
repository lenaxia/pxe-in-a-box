// Command pxe-gen reads machine, asset, menu, and secrets config, validates
// them, renders Talos machine configs from templates, and generates matchbox
// groups, profiles, and the boot.ipxe script.
//
// This command runs inside the container at startup (and can also run
// standalone for testing/debugging).
//
// Usage:
//
//	pxe-gen --config-dir /config --assets-dir /assets \
//	  --addr 192.168.1.100 --port 8081
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lenaxia/pxe-in-a-box/internal/bootscript"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/matchbox"
	"github.com/lenaxia/pxe-in-a-box/internal/renderer"
)

func main() {
	var (
		configDir string
		assetsDir string
		addr      string
		port      int
	)

	flag.StringVar(&configDir, "config-dir", "/config", "configuration directory")
	flag.StringVar(&assetsDir, "assets-dir", "/assets", "assets directory")
	flag.StringVar(&addr, "addr", "", "matchbox HTTP address, e.g. 192.168.1.100 (required)")
	flag.IntVar(&port, "port", 8081, "matchbox HTTP port")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pxe-gen renders templates and generates matchbox configs.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: pxe-gen [flags]\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if addr == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := run(configDir, assetsDir, addr, port); err != nil {
		log.Fatalf("pxe-gen: %v", err)
	}
}

func run(configDir, assetsDir, addr string, port int) error {
	machinesPath := filepath.Join(configDir, "machines.yaml")
	assetsPath := filepath.Join(configDir, "assets.yaml")
	menuPath := filepath.Join(configDir, "menu.yaml")
	templateDir := filepath.Join(configDir, "templates")
	renderedDir := filepath.Join(assetsDir, "rendered")

	// ── Load configs ─────────────────────────────────────────────────
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

	// ── Render Talos machine configs ──────────────────────────────────
	secrets, err := renderer.LoadSecrets(configDir)
	if err != nil {
		return fmt.Errorf("loading secrets: %w", err)
	}

	if secrets != nil && len(machines.Groups) > 0 {
		engine := renderer.NewEngine(templateDir)

		var specs []renderer.RenderSpec
		for _, m := range machines.AllMachines() {
			// Skip singletons — they use static config files, no rendering
			if m.Config != "" {
				continue
			}
			if m.Template == "" {
				continue
			}

			// Infer role from template filename
			role := renderer.RoleWorker
			if strings.Contains(m.Template, "controlplane") {
				role = renderer.RoleControlplane
			}

			data := renderer.BuildTemplateData(m.Hostname, m.Vars, secrets)

			specs = append(specs, renderer.RenderSpec{
				Hostname: m.Hostname,
				Template: m.Template,
				Role:     role,
				Data:     data,
			})
		}

		if len(specs) > 0 {
			if err := engine.RenderAll(specs, renderedDir); err != nil {
				return fmt.Errorf("rendering templates: %w", err)
			}
			log.Printf("rendered %d machine configs in %s", len(specs), renderedDir)
		}
	} else {
		log.Printf("skipping template rendering (no secrets file or no groups)")
	}

	// ── Generate matchbox groups and profiles ────────────────────────
	endpoint := matchbox.Endpoint{Address: addr, Port: port}

	groupsDir := filepath.Join(configDir, "groups")
	profilesDir := filepath.Join(configDir, "profiles")

	if err := matchbox.GenerateGroups(machines, groupsDir); err != nil {
		return fmt.Errorf("generating groups: %w", err)
	}
	log.Printf("generated %d group files in %s", len(machines.AllMachines()), groupsDir)

	if err := matchbox.GenerateProfiles(machines, assets, endpoint, profilesDir); err != nil {
		return fmt.Errorf("generating profiles: %w", err)
	}
	log.Printf("generated %d profile files in %s", len(machines.AllMachines()), profilesDir)

	// ── Generate boot.ipxe ───────────────────────────────────────────
	bootPath := filepath.Join(assetsDir, "boot.ipxe")
	if err := bootscript.Generate(menu, assets, bootscript.Endpoint(endpoint), bootPath); err != nil {
		return fmt.Errorf("generating boot.ipxe: %w", err)
	}
	log.Printf("generated boot.ipxe at %s", bootPath)

	return nil
}
