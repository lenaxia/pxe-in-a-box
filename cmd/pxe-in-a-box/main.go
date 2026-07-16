// Command pxe-in-a-box is the container entrypoint. It:
//  1. Validates configuration
//  2. Downloads missing assets
//  3. Runs optional cleanup
//  4. Starts dnsmasq and matchbox via s6-overlay
//
// This command runs inside the Docker container on the PXE host.
//
// Usage:
//
//	pxe-in-a-box [--config-dir /config] [--assets-dir /assets] [--skip-download]
//	             [--cleanup] [--dry-run] [--dump-state]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/homelab/pxe-in-a-box/internal/cleanup"
	"github.com/homelab/pxe-in-a-box/internal/config"
	"github.com/homelab/pxe-in-a-box/internal/downloader"
)

func main() {
	var (
		configDir    string
		assetsDir    string
		skipDownload bool
		cleanupFlag  bool
		dryRun       bool
		dumpState    bool
		logLevel     string
	)

	flag.StringVar(&configDir, "config-dir", "/config", "configuration directory")
	flag.StringVar(&assetsDir, "assets-dir", "/assets", "assets directory")
	flag.BoolVar(&skipDownload, "skip-download", false, "skip asset download phase")
	flag.BoolVar(&cleanupFlag, "cleanup", false, "force cleanup of orphaned assets")
	flag.BoolVar(&dryRun, "dry-run", false, "dry-run mode: log actions without making changes")
	flag.BoolVar(&dumpState, "dump-state", false, "print current state (groups, profiles, assets) and exit")
	flag.StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	flag.Parse()

	if dumpState {
		dumpCurrentState(configDir, assetsDir)
		return
	}

	if err := run(configDir, assetsDir, skipDownload, cleanupFlag, dryRun); err != nil {
		log.Fatalf("pxe-in-a-box: %v", err)
	}
}

func run(configDir, assetsDir string, skipDownload, cleanupFlag, dryRun bool) error {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Phase 1: Load and validate config
	_, assets, _, err := loadAndValidate(configDir)
	if err != nil {
		return err
	}

	// Phase 2: Download missing assets
	if !skipDownload {
		logger.Println("phase 2: downloading assets")
		dl := &downloader.Downloader{
			AssetsDir:  assetsDir,
			Client:     downloader.DefaultClient(),
			MaxRetries: 3,
			Log:        logger,
		}

		specs := downloader.ResolveAssetSpecs(assets)
		summary := dl.DownloadAll(specs)

		logger.Printf("assets: %d downloaded, %d skipped, %d failed",
			summary.Downloaded, summary.Skipped, summary.Failed)

		for _, result := range summary.Results {
			for _, fr := range result.Files {
				if fr.Error != nil {
					logger.Printf("  [warn] %s/%s: %v", result.Spec.ID, fr.Filename, fr.Error)
				}
			}
		}
	} else {
		logger.Println("phase 2: skipping asset download (--skip-download)")
	}

	// Phase 3: Optional cleanup
	if cleanup.ShouldCleanup(assets, cleanupFlag) && !dryRun {
		logger.Println("phase 3: cleaning up orphaned assets")
		cl := &cleanup.Cleaner{
			AssetsDir: assetsDir,
			Log:       logger,
		}
		results := cl.Run(assets)
		logger.Printf("cleanup: removed %d orphaned directories", len(results))
	} else if dryRun && cleanupFlag {
		logger.Println("phase 3: cleanup dry-run")
		cl := &cleanup.Cleaner{
			AssetsDir: assetsDir,
			Log:       logger,
			DryRun:    true,
		}
		cl.Run(assets)
	}

	// Phase 4: Start services
	logger.Println("phase 4: starting services")
	if err := startServices(configDir, assetsDir); err != nil {
		return fmt.Errorf("starting services: %w", err)
	}

	return nil
}

func loadAndValidate(configDir string) (*config.MachinesConfig, *config.AssetsConfig, *config.MenuConfig, error) {
	machinesPath := fmt.Sprintf("%s/machines.yaml", configDir)
	assetsPath := fmt.Sprintf("%s/assets.yaml", configDir)
	menuPath := fmt.Sprintf("%s/menu.yaml", configDir)

	machines, err := config.LoadMachines(machinesPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading machines: %w", err)
	}

	assets, err := config.LoadAssets(assetsPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading assets: %w", err)
	}

	menu, err := config.LoadMenu(menuPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading menu: %w", err)
	}

	fc := &config.FullConfig{Machines: machines, Assets: assets, Menu: menu}
	if err := config.Validate(fc); err != nil {
		return nil, nil, nil, fmt.Errorf("validation:\n%w", err)
	}

	return machines, assets, menu, nil
}

// startServices starts dnsmasq and matchbox as child processes and
// waits for them. This is used when s6-overlay is not available.
// In the Docker container, s6-overlay manages these processes instead.
func startServices(configDir, assetsDir string) error {
	// Start dnsmasq
	dnsmasq := exec.Command("dnsmasq", "--no-daemon", "--conf-file="+configDir+"/dnsmasq.conf")
	dnsmasq.Stdout = os.Stdout
	dnsmasq.Stderr = os.Stderr
	if err := dnsmasq.Start(); err != nil {
		return fmt.Errorf("starting dnsmasq: %w", err)
	}

	// Start matchbox
	matchbox := exec.Command("matchbox",
		"-address", "0.0.0.0:8081",
		"-data-path", configDir,
		"-assets-path", assetsDir,
		"-log-level", "info",
	)
	matchbox.Stdout = os.Stdout
	matchbox.Stderr = os.Stderr
	if err := matchbox.Start(); err != nil {
		return fmt.Errorf("starting matchbox: %w", err)
	}

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	doneCh := make(chan error, 2)
	go func() { doneCh <- dnsmasq.Wait() }()
	go func() { doneCh <- matchbox.Wait() }()

	select {
	case sig := <-sigCh:
		log.Printf("received signal %v, shutting down", sig)
		dnsmasq.Process.Signal(syscall.SIGTERM)
		matchbox.Process.Signal(syscall.SIGTERM)
		return nil
	case err := <-doneCh:
		if err != nil {
			return fmt.Errorf("service exited: %w", err)
		}
		return nil
	}
}

func dumpCurrentState(configDir, assetsDir string) {
	fmt.Printf("=== PXE-in-a-Box State ===\n\n")

	// List groups
	groupsDir := configDir + "/groups"
	fmt.Printf("Groups (%s):\n", groupsDir)
	if entries, err := os.ReadDir(groupsDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}

	// List profiles
	profilesDir := configDir + "/profiles"
	fmt.Printf("\nProfiles (%s):\n", profilesDir)
	if entries, err := os.ReadDir(profilesDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}

	// List assets
	fmt.Printf("\nAssets (%s):\n", assetsDir)
	if entries, err := os.ReadDir(assetsDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}
}
