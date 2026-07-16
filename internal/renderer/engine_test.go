package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validControlplaneData returns a TemplateData with all required fields set.
// Tests use this as a base and mutate individual fields to test validation.
func validControlplaneData() *TemplateData {
	return &TemplateData{
		Hostname:        "cp-00",
		NodeIP:          "192.168.1.10",
		ClusterName:     "home-kubernetes",
		ClusterEndpoint: "192.168.1.30",
		MachineToken:    "machine-token-value",
		MachineCACert:   "MACHINE_CA_CERT_BASE64",
		MachineCAKey:    "MACHINE_CA_KEY_BASE64",
		ClusterID:       "cluster-id-value",
		ClusterSecret:   "cluster-secret-value",
		ClusterToken:    "cluster-token-value",
		SecretboxKey:    "secretbox-value",
		ClusterCACert:   "CLUSTER_CA_CERT_BASE64",
		ClusterCAKey:    "CLUSTER_CA_KEY_BASE64",
		AggregatorCert:  "AGG_CERT_BASE64",
		AggregatorKey:   "AGG_KEY_BASE64",
		ServiceAcctKey:  "SA_KEY_BASE64",
		EtcdCACert:      "ETCD_CA_CERT_BASE64",
		EtcdCAKey:       "ETCD_CA_KEY_BASE64",
		InstallDisk:     "/dev/nvme0n1",
		InstallerImage:  "factory.talos.dev/metal-installer/abc123:v1.12.4",
	}
}

func validWorkerData() *TemplateData {
	return &TemplateData{
		Hostname:            "worker-01",
		NodeIP:              "192.168.1.20",
		ClusterName:         "home-kubernetes",
		ClusterEndpoint:     "192.168.1.30",
		MachineToken:        "machine-token-value",
		MachineCACert:       "MACHINE_CA_CERT_BASE64",
		ClusterID:           "cluster-id-value",
		ClusterSecret:       "cluster-secret-value",
		ClusterToken:        "cluster-token-value",
		ClusterCACert:       "CLUSTER_CA_CERT_BASE64",
		InstallDiskSelector: "naa.5002538e4019fdec",
		InstallerImage:      "factory.talos.dev/metal-installer/def456:v1.12.4",
	}
}

// ── Validation tests ─────────────────────────────────────────────────

func TestValidate_ControlplaneAllFieldsPresent(t *testing.T) {
	td := validControlplaneData()
	if err := td.Validate(RoleControlplane); err != nil {
		t.Fatalf("expected no error for valid CP data: %v", err)
	}
}

func TestValidate_WorkerAllFieldsPresent(t *testing.T) {
	td := validWorkerData()
	if err := td.Validate(RoleWorker); err != nil {
		t.Fatalf("expected no error for valid worker data: %v", err)
	}
}

func TestValidate_ControlplaneMissingCAKey(t *testing.T) {
	td := validControlplaneData()
	td.MachineCAKey = ""
	err := td.Validate(RoleControlplane)
	if err == nil {
		t.Fatal("expected error for missing machine_ca_key on controlplane")
	}
	if !strings.Contains(err.Error(), "machine_ca_key") {
		t.Errorf("error should mention machine_ca_key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cp-00") {
		t.Errorf("error should mention hostname, got: %v", err)
	}
}

func TestValidate_WorkerMissingCAKeyAllowed(t *testing.T) {
	td := validWorkerData()
	td.MachineCAKey = ""
	if err := td.Validate(RoleWorker); err != nil {
		t.Fatalf("worker should not require machine_ca_key: %v", err)
	}
}

func TestValidate_MissingMultipleFields(t *testing.T) {
	td := validControlplaneData()
	td.MachineToken = ""
	td.ClusterID = ""
	td.EtcdCACert = ""
	td.InstallDisk = ""
	td.InstallDiskSelector = ""

	err := td.Validate(RoleControlplane)
	if err == nil {
		t.Fatal("expected error for multiple missing fields")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if len(ve.Errors) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(ve.Errors), ve.Errors)
	}

	errStr := err.Error()
	for _, expected := range []string{"machine_token", "cluster_id", "etcd_ca_cert", "install"} {
		if !strings.Contains(errStr, expected) {
			t.Errorf("error should mention %q, got: %v", expected, err)
		}
	}
}

func TestValidate_MissingHostname(t *testing.T) {
	td := validControlplaneData()
	td.Hostname = ""
	err := td.Validate(RoleControlplane)
	if err == nil {
		t.Fatal("expected error for missing hostname")
	}
	if !strings.Contains(err.Error(), "hostname") {
		t.Errorf("error should mention hostname, got: %v", err)
	}
}

func TestValidate_NoInstallTarget(t *testing.T) {
	td := validWorkerData()
	td.InstallDisk = ""
	td.InstallDiskSelector = ""
	err := td.Validate(RoleWorker)
	if err == nil {
		t.Fatal("expected error when no install disk or selector")
	}
	if !strings.Contains(err.Error(), "install_disk or install_disk_selector") {
		t.Errorf("error should explain install requirement, got: %v", err)
	}
}

func TestValidate_ControlplaneRequiresAllSecrets(t *testing.T) {
	secretFields := []struct {
		field string
		set   func(td *TemplateData)
	}{
		{"machine_ca_key", func(td *TemplateData) { td.MachineCAKey = "" }},
		{"cluster_ca_key", func(td *TemplateData) { td.ClusterCAKey = "" }},
		{"aggregator_ca_cert", func(td *TemplateData) { td.AggregatorCert = "" }},
		{"aggregator_ca_key", func(td *TemplateData) { td.AggregatorKey = "" }},
		{"service_account_key", func(td *TemplateData) { td.ServiceAcctKey = "" }},
		{"etcd_ca_cert", func(td *TemplateData) { td.EtcdCACert = "" }},
		{"etcd_ca_key", func(td *TemplateData) { td.EtcdCAKey = "" }},
		{"secretbox_key", func(td *TemplateData) { td.SecretboxKey = "" }},
	}

	for _, sf := range secretFields {
		t.Run(sf.field, func(t *testing.T) {
			td := validControlplaneData()
			sf.set(td)
			err := td.Validate(RoleControlplane)
			if err == nil {
				t.Errorf("controlplane should require %s", sf.field)
			}
		})
	}
}

func TestValidate_WorkerDoesNotRequireControlplaneSecrets(t *testing.T) {
	td := validWorkerData()
	td.MachineCAKey = ""
	td.ClusterCAKey = ""
	td.AggregatorCert = ""
	td.AggregatorKey = ""
	td.ServiceAcctKey = ""
	td.EtcdCACert = ""
	td.EtcdCAKey = ""
	td.SecretboxKey = ""

	if err := td.Validate(RoleWorker); err != nil {
		t.Fatalf("worker should not require CP secrets: %v", err)
	}
}

func TestValidationError_Format(t *testing.T) {
	ve := &ValidationError{Hostname: "worker-01"}
	ve.add("node_ip", "required but not set")
	ve.add("cluster_id", "required but not set")

	errStr := ve.Error()
	if !strings.Contains(errStr, "worker-01") {
		t.Error("should contain hostname")
	}
	if !strings.Contains(errStr, "1. node_ip") {
		t.Error("should number errors")
	}
	if !strings.Contains(errStr, "2. cluster_id") {
		t.Error("should number errors sequentially")
	}
}

// ── Template rendering tests ─────────────────────────────────────────

func setupTestTemplates(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Write minimal templates for testing
	cpTemplate := `type: controlplane
hostname: {{ .Hostname }}
ip: {{ .NodeIP }}/16
token: {{ .MachineToken }}
ca_crt: {{ .MachineCACert }}
ca_key: {{ .MachineCAKey }}
endpoint: {{ .ClusterEndpoint }}
`

	workerTemplate := `type: worker
hostname: {{ .Hostname }}
ip: {{ .NodeIP }}/16
token: {{ .MachineToken }}
{{ if .IsGPU }}gpu: true
{{ end }}{{ if .InstallDiskSelector }}disk: {{ .InstallDiskSelector }}{{ else }}disk: {{ .InstallDisk }}{{ end }}
`

	os.WriteFile(filepath.Join(dir, "controlplane.yaml.tmpl"), []byte(cpTemplate), 0644)
	os.WriteFile(filepath.Join(dir, "worker.yaml.tmpl"), []byte(workerTemplate), 0644)

	return dir
}

func TestRender_Controlplane(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	td := validControlplaneData()

	result, err := engine.Render("controlplane.yaml.tmpl", RoleControlplane, td)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(result.Output, "type: controlplane") {
		t.Error("should contain machine type")
	}
	if !strings.Contains(result.Output, "hostname: cp-00") {
		t.Error("should contain hostname")
	}
	if !strings.Contains(result.Output, "192.168.1.10/16") {
		t.Error("should contain node IP with CIDR")
	}
	if !strings.Contains(result.Output, "machine-token-value") {
		t.Error("should contain machine token")
	}
}

func TestRender_Worker(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	td := validWorkerData()

	result, err := engine.Render("worker.yaml.tmpl", RoleWorker, td)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(result.Output, "type: worker") {
		t.Error("should contain machine type")
	}
	if !strings.Contains(result.Output, "disk: naa.5002538e4019fdec") {
		t.Error("should contain disk selector")
	}
	if strings.Contains(result.Output, "gpu: true") {
		t.Error("should NOT have GPU section by default")
	}
}

func TestRender_WorkerGPU(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	td := validWorkerData()
	td.IsGPU = true

	result, err := engine.Render("worker.yaml.tmpl", RoleWorker, td)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(result.Output, "gpu: true") {
		t.Error("should contain GPU section when IsGPU=true")
	}
}

func TestRender_TemplateNotFound(t *testing.T) {
	engine := NewEngine(t.TempDir())
	td := validControlplaneData()

	_, err := engine.Render("nonexistent.tmpl", RoleControlplane, td)
	if err == nil {
		t.Fatal("expected error for missing template file")
	}
	if !strings.Contains(err.Error(), "loading template") {
		t.Errorf("error should mention loading template, got: %v", err)
	}
}

func TestRender_InvalidTemplateSyntax(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.tmpl"), []byte("{{ .Hostname }"), 0644)
	engine := NewEngine(dir)

	_, err := engine.Render("bad.tmpl", RoleControlplane, validControlplaneData())
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

func TestRender_ValidationFailsBeforeRender(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	td := validControlplaneData()
	td.MachineToken = ""

	_, err := engine.Render("controlplane.yaml.tmpl", RoleControlplane, td)
	if err == nil {
		t.Fatal("expected validation error before rendering")
	}
	if !strings.Contains(err.Error(), "machine_token") {
		t.Errorf("error should identify missing field, got: %v", err)
	}
}

func TestRenderToFile(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	td := validControlplaneData()

	outputPath := filepath.Join(t.TempDir(), "subdir", "cp-00.yaml")
	err := engine.RenderToFile("controlplane.yaml.tmpl", RoleControlplane, td, outputPath)
	if err != nil {
		t.Fatalf("RenderToFile: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	if !strings.Contains(string(data), "cp-00") {
		t.Error("file should contain rendered content")
	}
}

func TestRenderAll_AllValid(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	outputDir := filepath.Join(t.TempDir(), "rendered")

	specs := []RenderSpec{
		{"cp-00", "controlplane.yaml.tmpl", RoleControlplane, validControlplaneData()},
		{"worker-01", "worker.yaml.tmpl", RoleWorker, validWorkerData()},
	}

	if err := engine.RenderAll(specs, outputDir); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	for _, h := range []string{"cp-00", "worker-01"} {
		path := filepath.Join(outputDir, h+".yaml")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist", path)
		}
	}
}

func TestRenderAll_AbortsOnValidationError(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	outputDir := filepath.Join(t.TempDir(), "rendered")

	// First spec valid, second invalid
	badData := validWorkerData()
	badData.MachineToken = ""

	specs := []RenderSpec{
		{"worker-01", "worker.yaml.tmpl", RoleWorker, validWorkerData()},
		{"worker-02", "worker.yaml.tmpl", RoleWorker, badData},
	}

	err := engine.RenderAll(specs, outputDir)
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Nothing should be written if validation fails
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		// Directory might exist (created by clearing) but should be empty
		entries, _ := os.ReadDir(outputDir)
		if len(entries) > 0 {
			t.Errorf("no files should be written on validation failure, found %d", len(entries))
		}
	}
}

func TestRenderAll_ClearsStaleFiles(t *testing.T) {
	tmplDir := setupTestTemplates(t)
	engine := NewEngine(tmplDir)
	outputDir := filepath.Join(t.TempDir(), "rendered")

	// Pre-create a stale file
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "old-machine.yaml"), []byte("stale"), 0644)

	specs := []RenderSpec{
		{"worker-01", "worker.yaml.tmpl", RoleWorker, validWorkerData()},
	}

	if err := engine.RenderAll(specs, outputDir); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "old-machine.yaml")); !os.IsNotExist(err) {
		t.Error("stale file should be removed")
	}
}

// ── BuildTemplateData tests ──────────────────────────────────────────

func TestBuildTemplateData_MergesVars(t *testing.T) {
	secrets := map[string]string{
		"machine_token":   "secret-token",
		"machine_ca_cert": "secret-cert",
		"cluster_id":      "cluster-id",
	}

	vars := map[string]string{
		"node_ip":         "192.168.1.10",
		"cluster_name":    "home-kubernetes",
		"install_disk":    "/dev/nvme0n1",
		"installer_image": "factory.talos.dev/metal-installer/abc:v1.12.4",
	}

	td := BuildTemplateData("cp-00", vars, secrets)

	if td.Hostname != "cp-00" {
		t.Errorf("hostname = %q", td.Hostname)
	}
	if td.MachineToken != "secret-token" {
		t.Errorf("machine_token should come from secrets, got %q", td.MachineToken)
	}
	if td.NodeIP != "192.168.1.10" {
		t.Errorf("node_ip should come from vars, got %q", td.NodeIP)
	}
}

func TestBuildTemplateData_VarsOverrideSecrets(t *testing.T) {
	secrets := map[string]string{
		"cluster_name": "from-secret",
	}
	vars := map[string]string{
		"cluster_name": "from-vars",
	}

	td := BuildTemplateData("host", vars, secrets)
	if td.ClusterName != "from-vars" {
		t.Errorf("vars should override secrets, got %q", td.ClusterName)
	}
}

func TestBuildTemplateData_BoolParsing(t *testing.T) {
	vars := map[string]string{
		"is_gpu":    "true",
		"wipe_disk": "true",
	}

	td := BuildTemplateData("host", vars, nil)

	if !td.IsGPU {
		t.Error("is_gpu=true should set IsGPU")
	}
	if !td.WipeDisk {
		t.Error("wipe_disk=true should set WipeDisk")
	}
}

func TestBuildTemplateData_Nameservers(t *testing.T) {
	vars := map[string]string{
		"nameservers": "8.8.8.8, 1.1.1.1, 9.9.9.9",
	}

	td := BuildTemplateData("host", vars, nil)

	if len(td.Nameservers) != 3 {
		t.Fatalf("expected 3 nameservers, got %d", len(td.Nameservers))
	}
	if td.Nameservers[0] != "8.8.8.8" {
		t.Errorf("nameserver[0] = %q", td.Nameservers[0])
	}
	if td.Nameservers[1] != "1.1.1.1" {
		t.Errorf("nameserver[1] = %q (should be trimmed)", td.Nameservers[1])
	}
}

func TestBuildTemplateData_EmptyMaps(t *testing.T) {
	td := BuildTemplateData("host", nil, nil)
	if td.Hostname != "host" {
		t.Errorf("hostname = %q", td.Hostname)
	}
}
