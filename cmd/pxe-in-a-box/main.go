// Command pxe-in-a-box is the container entrypoint. It:
//  1. Renders Talos machine configs from templates (pxe-gen)
//  2. Validates configuration
//  3. Downloads missing assets
//  4. Runs optional cleanup
//  5. Starts dnsmasq and matchbox
//
// Usage:
//
//	pxe-in-a-box [--config-dir /config] [--assets-dir /assets] [--skip-download]
//	             [--cleanup] [--dry-run] [--dump-state]
//	             [--addr 192.168.1.100] [--port 8081]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/lenaxia/pxe-in-a-box/internal/cleanup"
	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/downloader"
)

func main() {
	var (
		configDir    string
		assetsDir    string
		addr         string
		port         int
		skipDownload bool
		skipRender   bool
		cleanupFlag  bool
		dryRun       bool
		dumpState    bool
		logLevel     string
	)

	flag.StringVar(&configDir, "config-dir", "/config", "configuration directory")
	flag.StringVar(&assetsDir, "assets-dir", "/assets", "assets directory")
	flag.StringVar(&addr, "addr", "192.168.1.100", "matchbox HTTP address (for generated URLs)")
	flag.IntVar(&port, "port", 8081, "matchbox HTTP port")
	flag.BoolVar(&skipDownload, "skip-download", false, "skip asset download phase")
	flag.BoolVar(&skipRender, "skip-render", false, "skip template rendering phase")
	flag.BoolVar(&cleanupFlag, "cleanup", false, "force cleanup of orphaned assets")
	flag.BoolVar(&dryRun, "dry-run", false, "dry-run mode: log actions without making changes")
	flag.BoolVar(&dumpState, "dump-state", false, "print current state (groups, profiles, assets) and exit")
	flag.StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	flag.Parse()

	if dumpState {
		dumpCurrentState(configDir, assetsDir)
		return
	}

	if err := run(configDir, assetsDir, addr, port, skipDownload, skipRender, cleanupFlag, dryRun); err != nil {
		log.Fatalf("pxe-in-a-box: %v", err)
	}
}

func run(configDir, assetsDir, addr string, port int, skipDownload, skipRender, cleanupFlag, dryRun bool) error {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Phase 1: Render templates and generate matchbox configs
	if !skipRender {
		logger.Println("phase 1: rendering templates and generating configs")
		if err := runPXEGen(configDir, assetsDir, addr, port); err != nil {
			return fmt.Errorf("phase 1: %w", err)
		}
	} else {
		logger.Println("phase 1: skipping rendering (--skip-render)")
	}

	// Phase 2: Download missing assets
	if !skipDownload {
		logger.Println("phase 2: downloading assets")
		_, assets, err := loadConfigs(configDir)
		if err != nil {
			return err
		}

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
	if !dryRun {
		_, assets, _ := loadConfigs(configDir)
		if cleanup.ShouldCleanup(assets, cleanupFlag) {
			logger.Println("phase 3: cleaning up orphaned assets")
			cl := &cleanup.Cleaner{
				AssetsDir: assetsDir,
				Log:       logger,
			}
			results := cl.Run(assets)
			logger.Printf("cleanup: removed %d orphaned directories", len(results))
		}
	} else if cleanupFlag {
		logger.Println("phase 3: cleanup dry-run")
		_, assets, _ := loadConfigs(configDir)
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

// runPXEGen invokes pxe-gen to render templates and generate matchbox configs.
func runPXEGen(configDir, assetsDir, addr string, port int) error {
	cmd := exec.Command("pxe-gen",
		"--config-dir", configDir,
		"--assets-dir", assetsDir,
		"--addr", addr,
		"--port", fmt.Sprintf("%d", port),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func loadConfigs(configDir string) (*config.MachinesConfig, *config.AssetsConfig, error) {
	machinesPath := fmt.Sprintf("%s/machines.yaml", configDir)
	assetsPath := fmt.Sprintf("%s/assets.yaml", configDir)

	machines, err := config.LoadMachines(machinesPath)
	if err != nil {
		return nil, nil, err
	}

	assets, err := config.LoadAssets(assetsPath)
	if err != nil {
		return nil, nil, err
	}

	return machines, assets, nil
}

func startServices(configDir, assetsDir string) error {
	dnsmasq := exec.Command("dnsmasq", "--no-daemon", "--conf-file="+configDir+"/dnsmasq.conf")
	dnsmasq.Stdout = os.Stdout
	dnsmasq.Stderr = os.Stderr
	if err := dnsmasq.Start(); err != nil {
		return fmt.Errorf("starting dnsmasq: %w", err)
	}

	mb := exec.Command("matchbox",
		"-address", "0.0.0.0:8081",
		"-data-path", configDir,
		"-assets-path", assetsDir,
		"-log-level", "info",
	)
	mb.Stdout = os.Stdout
	mb.Stderr = os.Stderr
	if err := mb.Start(); err != nil {
		return fmt.Errorf("starting matchbox: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	doneCh := make(chan error, 2)
	go func() { doneCh <- dnsmasq.Wait() }()
	go func() { doneCh <- mb.Wait() }()

	select {
	case sig := <-sigCh:
		log.Printf("received signal %v, shutting down", sig)
		dnsmasq.Process.Signal(syscall.SIGTERM)
		mb.Process.Signal(syscall.SIGTERM)
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

	groupsDir := configDir + "/groups"
	fmt.Printf("Groups (%s):\n", groupsDir)
	if entries, err := os.ReadDir(groupsDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}

	profilesDir := configDir + "/profiles"
	fmt.Printf("\nProfiles (%s):\n", profilesDir)
	if entries, err := os.ReadDir(profilesDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}

	fmt.Printf("\nAssets (%s):\n", assetsDir)
	if entries, err := os.ReadDir(assetsDir); err == nil {
		for _, e := range entries {
			fmt.Printf("  %s\n", e.Name())
		}
	} else {
		fmt.Printf("  (none or directory not found)\n")
	}
}
