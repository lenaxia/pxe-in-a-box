package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Downloader fetches OS boot assets from upstream sources.
type Downloader struct {
	AssetsDir  string       // Root directory for assets (e.g., /assets)
	Client     *http.Client // HTTP client (configurable for testing)
	MaxRetries int          // Download retry count
	Log        *log.Logger  // Logger for progress output
}

// FileResult records the outcome of downloading a single file.
type FileResult struct {
	Filename string
	Size     int64
	Skipped  bool // true if file already existed and was valid
	Error    error
}

// AssetResult records the outcome of downloading all files for an asset.
type AssetResult struct {
	Spec  AssetSpec
	Files []FileResult
}

// Summary aggregates download results for the startup log.
type Summary struct {
	Downloaded int
	Skipped    int
	Failed     int
	Results    []AssetResult
}

// DownloadAll processes all asset specs, downloading missing files
// and verifying checksums where available. Returns a summary of results.
// A download failure on one asset does not prevent other assets from downloading.
func (d *Downloader) DownloadAll(specs []AssetSpec) *Summary {
	summary := &Summary{}

	for _, spec := range specs {
		result := d.downloadAsset(spec)
		summary.Results = append(summary.Results, result)

		for _, fr := range result.Files {
			if fr.Error != nil {
				summary.Failed++
			} else if fr.Skipped {
				summary.Skipped++
			} else {
				summary.Downloaded++
			}
		}
	}

	return summary
}

func (d *Downloader) downloadAsset(spec AssetSpec) AssetResult {
	result := AssetResult{Spec: spec}

	destDir := filepath.Join(d.AssetsDir, spec.BaseDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		for range spec.Files {
			result.Files = append(result.Files, FileResult{Error: fmt.Errorf("creating dir %s: %w", destDir, err)})
		}
		return result
	}

	checksums := d.fetchChecksums(spec)

	for _, fileSpec := range spec.Files {
		fr := d.downloadFile(destDir, fileSpec, checksums)
		result.Files = append(result.Files, fr)
	}

	return result
}

func (d *Downloader) downloadFile(destDir string, spec FileSpec, checksums map[string]string) FileResult {
	destPath := filepath.Join(destDir, spec.Filename)

	// Check if file already exists and has non-zero size
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		d.Log.Printf("  [skip] %s (%s) — already exists", spec.Filename, formatSize(info.Size()))
		return FileResult{Filename: spec.Filename, Size: info.Size(), Skipped: true}
	}

	// Determine expected SHA256
	expectedSHA := spec.SHA256
	if expectedSHA == "" && checksums != nil {
		// Try to match against checksum file entries
		for checksumFile, hash := range checksums {
			if strings.HasPrefix(checksumFile, spec.URL[strings.LastIndex(spec.URL, "/")+1:]) {
				expectedSHA = hash
				break
			}
		}
	}

	// Download with retries
	var lastErr error
	for attempt := 1; attempt <= d.MaxRetries; attempt++ {
		size, err := d.fetchAndSave(spec.URL, destPath)
		if err != nil {
			lastErr = err
			d.Log.Printf("  [retry %d/%d] %s — %v", attempt, d.MaxRetries, spec.Filename, err)
			if attempt < d.MaxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
			continue
		}

		// Verify checksum if we have one
		if expectedSHA != "" {
			actualSHA, err := sha256File(destPath)
			if err != nil {
				lastErr = fmt.Errorf("checksum read failed: %w", err)
				os.Remove(destPath)
				continue
			}
			if actualSHA != expectedSHA {
				lastErr = fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA[:12]+"...", actualSHA[:12]+"...")
				os.Remove(destPath)
				continue
			}
		}

		d.Log.Printf("  [ok] %s (%s)", spec.Filename, formatSize(size))
		return FileResult{Filename: spec.Filename, Size: size}
	}

	return FileResult{Filename: spec.Filename, Error: lastErr}
}

func (d *Downloader) fetchAndSave(url, destPath string) (int64, error) {
	resp, err := d.Client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("creating file: %w", err)
	}

	written, err := io.Copy(file, resp.Body)
	file.Close()
	if err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("writing file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("renaming temp file: %w", err)
	}

	return written, nil
}

func (d *Downloader) fetchChecksums(spec AssetSpec) map[string]string {
	if spec.ChecksumURL == "" {
		return nil
	}

	resp, err := d.Client.Get(spec.ChecksumURL)
	if err != nil {
		d.Log.Printf("  [warn] could not fetch checksums from %s: %v", spec.ChecksumURL, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.Log.Printf("  [warn] checksums HTTP %d from %s", resp.StatusCode, spec.ChecksumURL)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return parseSHA256Sums(string(body))
}

// parseSHA256Sums parses a sha256sum.txt file into a map of filename → hash.
func parseSHA256Sums(content string) map[string]string {
	checksums := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 66 { // SHA256 (64 chars) + space + space + filename minimum
			continue
		}
		// Format: "<64-hex-hash>  <filename>"
		hash := line[:64]
		filename := strings.TrimSpace(line[64:])
		if filename != "" {
			checksums[filename] = hash
		}
	}
	return checksums
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// DefaultClient returns an HTTP client suitable for large file downloads.
func DefaultClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
	}
}
