package downloader

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/homelab/pxe-in-a-box/internal/config"
)

// newTestDownloader creates a Downloader writing to a temp dir,
// using the given HTTP client (or a default one).
func newTestDownloader(t *testing.T, client *http.Client) (*Downloader, string) {
	t.Helper()
	dir := t.TempDir()
	if client == nil {
		client = DefaultClient()
	}
	return &Downloader{
		AssetsDir:  dir,
		Client:     client,
		MaxRetries: 2,
		Log:        log.New(os.Stderr, "", 0),
	}, dir
}

func TestParseSHA256Sums(t *testing.T) {
	content := `abc123def4567890abcdef1234567890abcdef1234567890abcdef1234567890  vmlinuz-amd64
fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321  initramfs-amd64.xz
`
	checksums := parseSHA256Sums(content)

	if len(checksums) != 2 {
		t.Fatalf("expected 2 checksums, got %d", len(checksums))
	}
	if checksums["vmlinuz-amd64"] != "abc123def4567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Errorf("unexpected vmlinuz checksum: %q", checksums["vmlinuz-amd64"])
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if !strings.HasPrefix(got, tt.want[:len(tt.want)-3]) {
			t.Errorf("formatSize(%d) = %q, want prefix of %q", tt.bytes, got, tt.want)
		}
	}
}

func TestResolveTalosOfficial(t *testing.T) {
	a := config.TalosAsset{
		ID:      "talos-v1.10.6",
		Version: "v1.10.6",
		Arch:    "amd64",
	}

	spec := resolveTalos(a)

	if spec.ID != "talos-v1.10.6" {
		t.Errorf("ID = %q", spec.ID)
	}
	if len(spec.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(spec.Files))
	}

	if !strings.Contains(spec.Files[0].URL, "github.com/siderolabs/talos/releases/download/v1.10.6/vmlinuz-amd64") {
		t.Errorf("kernel URL = %q", spec.Files[0].URL)
	}
	if spec.Files[0].Filename != "vmlinuz" {
		t.Errorf("kernel filename = %q", spec.Files[0].Filename)
	}
	if spec.Files[1].Filename != "initramfs.xz" {
		t.Errorf("initrd filename = %q", spec.Files[1].Filename)
	}
	if spec.ChecksumURL == "" {
		t.Error("expected non-empty checksum URL for official release")
	}
}

func TestResolveTalosImageFactory(t *testing.T) {
	a := config.TalosAsset{
		ID:               "talos-nvidia",
		ImageFactoryHash: "37f5a3fbd1e1e5d2a3c4",
		Version:          "v1.9.5",
		Arch:             "amd64",
		DownloadUKI:      true,
	}

	spec := resolveTalos(a)

	if spec.ChecksumURL != "" {
		t.Error("Image Factory assets should not have a checksum URL")
	}
	if len(spec.Files) != 3 {
		t.Fatalf("expected 3 files (kernel + initrd + UKI), got %d", len(spec.Files))
	}
	if !strings.Contains(spec.Files[0].URL, "factory.talos.dev/image/37f5a3fbd1e1e5d2a3c4/v1.9.5/kernel-amd64") {
		t.Errorf("kernel URL = %q", spec.Files[0].URL)
	}
	if spec.Files[2].Filename != "metal-amd64-uki.efi" {
		t.Errorf("UKI filename = %q", spec.Files[2].Filename)
	}
}

func TestDownloadFile_Success(t *testing.T) {
	content := []byte("fake kernel data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	dl, dir := newTestDownloader(t, server.Client())

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz"},
		},
	}

	result := dl.downloadAsset(spec)
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file result, got %d", len(result.Files))
	}
	if result.Files[0].Error != nil {
		t.Fatalf("download failed: %v", result.Files[0].Error)
	}
	if result.Files[0].Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", result.Files[0].Size, len(content))
	}

	// Verify file was written
	destPath := filepath.Join(dir, "assets/test/amd64/vmlinuz")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch")
	}
}

func TestDownloadFile_SkipsExisting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for existing file")
	}))
	defer server.Close()

	dl, dir := newTestDownloader(t, nil)

	// Pre-create the file
	destDir := filepath.Join(dir, "assets/test/amd64")
	os.MkdirAll(destDir, 0755)
	os.WriteFile(filepath.Join(destDir, "vmlinuz"), []byte("existing"), 0644)

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz"},
		},
	}

	result := dl.downloadAsset(spec)
	if !result.Files[0].Skipped {
		t.Error("expected file to be skipped")
	}
}

func TestDownloadFile_Retries(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			http.Error(w, "temporary error", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("success on third try"))
	}))
	defer server.Close()

	dl, _ := newTestDownloader(t, server.Client())
	dl.MaxRetries = 3

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz"},
		},
	}

	result := dl.downloadAsset(spec)
	if result.Files[0].Error != nil {
		t.Errorf("expected success after retries, got: %v", result.Files[0].Error)
	}
	if callCount != 3 {
		t.Errorf("expected 3 attempts, got %d", callCount)
	}
}

func TestDownloadFile_FailsAfterMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	dl, _ := newTestDownloader(t, server.Client())
	dl.MaxRetries = 2

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz"},
		},
	}

	result := dl.downloadAsset(spec)
	if result.Files[0].Error == nil {
		t.Error("expected error after max retries")
	}
}

func TestDownloadFile_SHA256Verification(t *testing.T) {
	content := []byte("kernel data for hashing")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	// Compute actual SHA256
	hash := computeSHA256(t, content)

	dl, _ := newTestDownloader(t, server.Client())

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz", SHA256: hash},
		},
	}

	result := dl.downloadAsset(spec)
	if result.Files[0].Error != nil {
		t.Fatalf("expected success with valid checksum, got: %v", result.Files[0].Error)
	}
}

func TestDownloadFile_SHA256Mismatch(t *testing.T) {
	content := []byte("kernel data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	dl, _ := newTestDownloader(t, server.Client())

	spec := AssetSpec{
		ID:      "test",
		BaseDir: "/assets/test/amd64",
		Files: []FileSpec{
			{URL: server.URL + "/vmlinuz", Filename: "vmlinuz", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
		},
	}

	result := dl.downloadAsset(spec)
	if result.Files[0].Error == nil {
		t.Error("expected checksum mismatch error")
	}
	if !strings.Contains(result.Files[0].Error.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", result.Files[0].Error)
	}
}

func TestDownloadAll_MultipleAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer server.Close()

	dl, _ := newTestDownloader(t, server.Client())

	specs := []AssetSpec{
		{
			ID:      "talos-1",
			BaseDir: "/assets/talos-1/amd64",
			Files: []FileSpec{
				{URL: server.URL + "/vmlinuz", Filename: "vmlinuz"},
				{URL: server.URL + "/initramfs.xz", Filename: "initramfs.xz"},
			},
		},
		{
			ID:      "ubuntu-1",
			BaseDir: "/assets/ubuntu-1/amd64",
			Files: []FileSpec{
				{URL: server.URL + "/linux", Filename: "linux"},
			},
		},
	}

	summary := dl.DownloadAll(specs)

	if summary.Downloaded != 3 {
		t.Errorf("expected 3 downloaded, got %d", summary.Downloaded)
	}
	if summary.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", summary.Failed)
	}
}

func TestDownloadAll_FailedAssetDoesNotBlockOthers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bad") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Write([]byte("good data"))
	}))
	defer server.Close()

	dl, _ := newTestDownloader(t, server.Client())

	specs := []AssetSpec{
		{
			ID:      "bad-asset",
			BaseDir: "/assets/bad/amd64",
			Files: []FileSpec{
				{URL: server.URL + "/bad/vmlinuz", Filename: "vmlinuz"},
			},
		},
		{
			ID:      "good-asset",
			BaseDir: "/assets/good/amd64",
			Files: []FileSpec{
				{URL: server.URL + "/good/vmlinuz", Filename: "vmlinuz"},
			},
		},
	}

	summary := dl.DownloadAll(specs)

	if summary.Downloaded != 1 {
		t.Errorf("expected 1 downloaded, got %d", summary.Downloaded)
	}
	if summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", summary.Failed)
	}
}

// Helpers

func computeSHA256(t *testing.T, data []byte) string {
	t.Helper()
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
