// Package cleanup removes asset directories that are no longer referenced
// in the asset manifest. This keeps the assets volume in sync with the
// declared configuration.
//
// Cleanup is opt-in (cleanup: true in assets.yaml or --cleanup flag).
// Protected paths (rendered/, static/, boot.ipxe) are never deleted.
package cleanup

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/homelab/pxe-in-a-box/internal/config"
)

// ProtectedPaths are directories/files inside the assets volume that must
// never be deleted by cleanup, even though they don't appear in the manifest.
var protectedPaths = map[string]bool{
	"rendered":  true,
	"static":    true,
	"boot.ipxe": true,
}

// Cleaner removes orphaned asset directories from the assets volume.
type Cleaner struct {
	AssetsDir string // Root assets directory
	Log       *log.Logger
	DryRun    bool // If true, log what would be deleted but don't delete
}

// DeleteResult records what was (or would be) deleted.
type DeleteResult struct {
	Path string
	Size int64
}

// Run scans the assets directory and removes any top-level directories
// that are not in the manifest's asset ID list and are not protected.
func (c *Cleaner) Run(cfg *config.AssetsConfig) []DeleteResult {
	validIDs := make(map[string]bool)
	for _, id := range cfg.AllAssetIDs() {
		validIDs[id] = true
	}

	entries, err := os.ReadDir(c.AssetsDir)
	if err != nil {
		c.Log.Printf("cleanup: cannot read assets dir %s: %v", c.AssetsDir, err)
		return nil
	}

	var results []DeleteResult

	for _, entry := range entries {
		name := entry.Name()

		if protectedPaths[name] {
			continue
		}
		if validIDs[name] {
			continue
		}

		fullPath := filepath.Join(c.AssetsDir, name)
		size := dirSize(fullPath)

		if c.DryRun {
			c.Log.Printf("  [dry-run] would delete %s (%s)", fullPath, formatSize(size))
		} else {
			c.Log.Printf("  [delete] %s (%s)", fullPath, formatSize(size))
			if err := os.RemoveAll(fullPath); err != nil {
				c.Log.Printf("  [error] failed to delete %s: %v", fullPath, err)
				continue
			}
		}

		results = append(results, DeleteResult{Path: fullPath, Size: size})
	}

	return results
}

// dirSize calculates the total size of all files in a directory tree.
func dirSize(path string) int64 {
	var size int64
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1073741824:
		return fmt.Sprintf("%.1f GB", float64(bytes)/1073741824)
	case bytes >= 1048576:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1048576)
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ShouldCleanup returns true if cleanup should run, based on the config
// and optional CLI override.
func ShouldCleanup(cfg *config.AssetsConfig, cliFlag bool) bool {
	return cfg.Cleanup || cliFlag
}

// IsProtected returns true if the given path name is protected from cleanup.
func IsProtected(name string) bool {
	return protectedPaths[strings.TrimPrefix(name, "/")]
}
