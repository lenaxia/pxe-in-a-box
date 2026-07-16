package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSecrets_Plaintext(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte(`
machine_token: "my-token"
cluster_id: "my-cluster-id"
machine_ca_cert: "base64-cert"
`), 0644)

	secrets, err := LoadSecrets(dir)
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}
	if secrets["machine_token"] != "my-token" {
		t.Errorf("machine_token = %q", secrets["machine_token"])
	}
	if secrets["cluster_id"] != "my-cluster-id" {
		t.Errorf("cluster_id = %q", secrets["cluster_id"])
	}
}

func TestLoadSecrets_NoSecretsFile(t *testing.T) {
	dir := t.TempDir()
	secrets, err := LoadSecrets(dir)
	if err != nil {
		t.Fatalf("should not error with no secrets file: %v", err)
	}
	if secrets != nil {
		t.Error("should return nil with no secrets file")
	}
}

func TestLoadSecrets_SOPSMissingAgeKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secrets.sops.yaml"), []byte("encrypted"), 0644)
	// No age.key

	_, err := LoadSecrets(dir)
	if err == nil {
		t.Fatal("expected error when SOPS file exists but no age.key")
	}
	if !strings.Contains(err.Error(), "age.key") {
		t.Errorf("error should mention age.key, got: %v", err)
	}
}

func TestLoadSecrets_SOPSBinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secrets.sops.yaml"), []byte("encrypted"), 0644)
	os.WriteFile(filepath.Join(dir, "age.key"), []byte("AGE-SECRET-KEY-..."), 0644)

	// Temporarily remove sops from PATH
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", originalPath)

	_, err := LoadSecrets(dir)
	if err == nil {
		t.Fatal("expected error when sops binary not available")
	}
	if !strings.Contains(err.Error(), "sops") {
		t.Errorf("error should mention sops, got: %v", err)
	}
}

func TestParseYAMLSecrets(t *testing.T) {
	data := []byte(`
machine_token: "abc123"
cluster_id: "xyz"
some_bool: true
some_int: 42
nested:
  key: value
`)
	result, err := parseYAMLSecrets(data)
	if err != nil {
		t.Fatalf("parseYAMLSecrets: %v", err)
	}

	if result["machine_token"] != "abc123" {
		t.Errorf("machine_token = %q", result["machine_token"])
	}
	if result["some_bool"] != "true" {
		t.Errorf("some_bool = %q", result["some_bool"])
	}
	// Nested values are skipped (not strings)
	if _, ok := result["nested"]; ok {
		t.Error("nested map should be skipped")
	}
}

func TestParseYAMLSecrets_Invalid(t *testing.T) {
	_, err := parseYAMLSecrets([]byte("not: [valid: yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
