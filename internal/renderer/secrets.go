package renderer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LoadSecrets loads secret values from the config directory.
// Resolution order (first match wins):
//  1. secrets.sops.yaml + age.key → SOPS decrypt
//  2. secrets.yaml → plaintext
//
// Returns nil if no secrets file exists (some machines may use static configs
// that don't need template rendering).
func LoadSecrets(configDir string) (map[string]string, error) {
	sopsPath := filepath.Join(configDir, "secrets.sops.yaml")
	plainPath := filepath.Join(configDir, "secrets.yaml")
	ageKeyPath := filepath.Join(configDir, "age.key")

	// Option 1: SOPS-encrypted
	if fileExists(sopsPath) {
		if !fileExists(ageKeyPath) {
			return nil, fmt.Errorf("found %s but no age.key found in %s — "+
				"mount the age key file to decrypt SOPS-encrypted secrets", sopsPath, configDir)
		}

		return decryptSOPS(sopsPath, ageKeyPath)
	}

	// Option 2: plaintext
	if fileExists(plainPath) {
		return loadYAMLMap(plainPath)
	}

	// No secrets file — caller can skip rendering
	return nil, nil
}

// decryptSOPS runs `sops --decrypt` with the age key and parses the output.
func decryptSOPS(sopsPath, ageKeyPath string) (map[string]string, error) {
	if _, err := exec.LookPath("sops"); err != nil {
		return nil, fmt.Errorf("secrets.sops.yaml found but 'sops' binary not available — " +
			"install sops in the container or use plaintext secrets.yaml instead")
	}

	cmd := exec.Command("sops", "--decrypt", "--age-key-file", ageKeyPath, sopsPath)
	output, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("sops decrypt of %s failed: %w%s", sopsPath, err, formatStderr(stderr))
	}

	return parseYAMLBytes(output)
}

func loadYAMLMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return parseYAMLBytes(data)
}

func parseYAMLBytes(data []byte) (map[string]string, error) {
	// Use the same yaml.v3 package as the rest of the codebase
	return parseYAMLSecrets(data)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func formatStderr(stderr string) string {
	if stderr == "" {
		return ""
	}
	return "\nstderr: " + stderr
}
