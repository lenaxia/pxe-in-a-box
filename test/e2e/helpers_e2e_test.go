//go:build e2e

package e2e

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lenaxia/pxe-in-a-box/internal/config"
	"github.com/lenaxia/pxe-in-a-box/internal/matchbox"
)

// matchboxInstance manages a real matchbox server process for e2e testing.
type matchboxInstance struct {
	cmd       *exec.Cmd
	port      int
	dataDir   string
	assetsDir string
	t         *testing.T
}

func startMatchbox(t *testing.T) *matchboxInstance {
	t.Helper()

	binary := findMatchbox(t)

	port := freePort(t)
	dataDir := t.TempDir()
	assetsDir := t.TempDir()

	// Create matchbox subdirs
	for _, sub := range []string{"groups", "profiles"} {
		os.MkdirAll(filepath.Join(dataDir, sub), 0755)
	}

	cmd := exec.Command(binary,
		"-address", fmt.Sprintf("127.0.0.1:%d", port),
		"-data-path", dataDir,
		"-assets-path", assetsDir,
		"-log-level", "debug",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting matchbox: %v", err)
	}

	// Wait for matchbox to be ready
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForReady(t, baseURL, 10*time.Second) {
		cmd.Process.Kill()
		t.Fatalf("matchbox did not become ready.\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}

	mi := &matchboxInstance{
		cmd:       cmd,
		port:      port,
		dataDir:   dataDir,
		assetsDir: assetsDir,
		t:         t,
	}

	t.Cleanup(func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
	})

	return mi
}

func (mi *matchboxInstance) baseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", mi.port)
}

func (mi *matchboxInstance) get(path string) (int, string) {
	resp, err := http.Get(mi.baseURL() + path)
	if err != nil {
		mi.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func (mi *matchboxInstance) writeFile(subdir, name, content string) {
	dir := filepath.Join(mi.dataDir, subdir)
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		mi.t.Fatalf("writing %s/%s: %v", subdir, name, err)
	}
}

func (mi *matchboxInstance) writeAsset(name, content string) {
	path := filepath.Join(mi.assetsDir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}

func (mi *matchboxInstance) reloadGroups(machines *config.MachinesConfig) {
	endpoint := matchbox.Endpoint{Address: "127.0.0.1", Port: mi.port}
	groupsDir := filepath.Join(mi.dataDir, "groups")
	profilesDir := filepath.Join(mi.dataDir, "profiles")
	assets := &config.AssetsConfig{
		Talos: []config.TalosAsset{{ID: "talos-v1.10.6", Version: "v1.10.6", Arch: "amd64"}},
	}
	matchbox.GenerateGroups(machines, groupsDir)
	matchbox.GenerateProfiles(machines, assets, endpoint, profilesDir)
}

func findMatchbox(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"matchbox", "../../bin/matchbox", "/usr/local/bin/matchbox"} {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, "bin", "matchbox")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("matchbox binary not found, skipping e2e test. Install with: make download-matchbox")
	return ""
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("getting free port: %v", err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForReady(t *testing.T, url string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
