//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/pxe-in-a-box/internal/renderer"
)

// TestRenderPipeline_RealTemplates renders the actual shipped templates
// with complete test data and verifies the output contains all expected
// Talos config sections and values.
func TestRenderPipeline_RealTemplates(t *testing.T) {
	// Use the real templates from the repo
	templateDir := findTemplateDir(t)

	engine := renderer.NewEngine(templateDir)

	// --- Controlplane ---
	t.Run("controlplane", func(t *testing.T) {
		data := fullControlplaneData()

		result, err := engine.Render("controlplane.yaml.tmpl", renderer.RoleControlplane, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}

		out := result.Output

		// Core machine fields
		assertContains(t, out, "type: controlplane")
		assertContains(t, out, "hostname: cp-00")
		assertContains(t, out, "192.168.1.10/16")
		assertContains(t, out, "vip:")
		assertContains(t, out, "ip: 192.168.1.30")

		// Secrets
		assertContains(t, out, "machine-token-value")
		assertContains(t, out, "machine-ca-cert-base64")
		assertContains(t, out, "machine-ca-key-base64")
		assertContains(t, out, "cluster-id-value")
		assertContains(t, out, "cluster-secret-value")
		assertContains(t, out, "cluster-token-value")
		assertContains(t, out, "cluster-ca-cert-base64")
		assertContains(t, out, "cluster-ca-key-base64")
		assertContains(t, out, "agg-cert-base64")
		assertContains(t, out, "agg-key-base64")
		assertContains(t, out, "sa-key-base64")
		assertContains(t, out, "etcd-ca-cert-base64")
		assertContains(t, out, "etcd-ca-key-base64")
		assertContains(t, out, "secretbox-value")

		// Install
		assertContains(t, out, "disk: /dev/nvme0n1")
		assertContains(t, out, "factory.talos.dev/metal-installer/test-cp-schematic:v1.12.4")

		// K8s components
		assertContains(t, out, "kubelet:v1.35.2")
		assertContains(t, out, "kube-apiserver:v1.35.2")
		assertContains(t, out, "kube-controller-manager:v1.35.2")
		assertContains(t, out, "kube-scheduler:v1.35.2")

		// Cluster settings
		assertContains(t, out, "clusterName: home-kubernetes")
		assertContains(t, out, "https://192.168.1.30:6443")
		assertContains(t, out, "name: none")
		assertContains(t, out, "allowSchedulingOnControlPlanes: true")
		assertContains(t, out, "kubernetesTalosAPIAccess")
		assertContains(t, out, "port: 7445")

		// Etcd
		assertContains(t, out, "listen-metrics-urls: http://0.0.0.0:2381")
	})

	// --- Worker (standard) ---
	t.Run("worker_standard", func(t *testing.T) {
		data := fullWorkerData()

		result, err := engine.Render("worker.yaml.tmpl", renderer.RoleWorker, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}

		out := result.Output

		assertContains(t, out, "type: worker")
		assertContains(t, out, "hostname: worker-01")
		assertContains(t, out, "192.168.1.20/16")
		assertContains(t, out, "naa.5002538e4019fdec")

		// NO VIP on workers
		if strings.Contains(out, "vip:") {
			t.Error("worker should NOT have VIP")
		}

		// NO CA private keys on workers
		if strings.Contains(out, "machine-ca-key") {
			t.Error("worker should NOT have machine CA key")
		}

		// Standard containerd config (not nvidia)
		assertContains(t, out, "enable_unprivileged_ports = true")

		// Mounts
		assertContains(t, out, "/var/mnt/longhorn")
		assertContains(t, out, "/var/openebs/local")

		// Longhorn volume
		assertContains(t, out, "UserVolumeConfig")
		assertContains(t, out, "disk.transport == 'nvme'")

		// No GPU sections
		if strings.Contains(out, "nvidia") {
			t.Error("standard worker should NOT have nvidia sections")
		}
	})

	// --- Worker (GPU) ---
	t.Run("worker_gpu", func(t *testing.T) {
		data := fullWorkerData()
		data.IsGPU = true

		result, err := engine.Render("worker.yaml.tmpl", renderer.RoleWorker, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}

		out := result.Output

		assertContains(t, out, "default_runtime_name = \"nvidia\"")
		assertContains(t, out, "name: nvidia")
		assertContains(t, out, "name: nvidia_uvm")
		assertContains(t, out, "name: nvidia_drm")
		assertContains(t, out, "name: nvidia_modeset")
		assertContains(t, out, "bpf_jit_harden")

		// Should NOT have standard containerd config
		if strings.Contains(out, "enable_unprivileged_ports") {
			t.Error("GPU worker should NOT have standard containerd config")
		}
	})

	// --- Worker with install disk (not selector) ---
	t.Run("worker_install_disk", func(t *testing.T) {
		data := fullWorkerData()
		data.InstallDisk = "/dev/sda"
		data.InstallDiskSelector = ""

		result, err := engine.Render("worker.yaml.tmpl", renderer.RoleWorker, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}

		assertContains(t, result.Output, "disk: /dev/sda")
		if strings.Contains(result.Output, "wwid:") {
			t.Error("should use disk: not wwid selector")
		}
	})

	// --- Nameservers ---
	t.Run("custom_nameservers", func(t *testing.T) {
		data := fullWorkerData()
		data.Nameservers = []string{"8.8.8.8", "1.1.1.1"}

		result, err := engine.Render("worker.yaml.tmpl", renderer.RoleWorker, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}

		assertContains(t, result.Output, "8.8.8.8")
		assertContains(t, result.Output, "1.1.1.1")
	})
}

// TestRenderPipeline_BuildAndRender uses BuildTemplateData (the merge function)
// with realistic vars + secrets maps, then renders and verifies.
func TestRenderPipeline_BuildAndRender(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := renderer.NewEngine(templateDir)

	secrets := map[string]string{
		"machine_token":       "merged-token",
		"machine_ca_cert":     "MERGED_CA_CERT",
		"machine_ca_key":      "MERGED_CA_KEY",
		"cluster_id":          "merged-cluster-id",
		"cluster_secret":      "merged-cluster-secret",
		"cluster_token":       "merged-cluster-token",
		"secretbox_key":       "merged-secretbox",
		"cluster_ca_cert":     "MERGED_CLUSTER_CERT",
		"cluster_ca_key":      "MERGED_CLUSTER_KEY",
		"aggregator_ca_cert":  "MERGED_AGG_CERT",
		"aggregator_ca_key":   "MERGED_AGG_KEY",
		"service_account_key": "MERGED_SA_KEY",
		"etcd_ca_cert":        "MERGED_ETCD_CERT",
		"etcd_ca_key":         "MERGED_ETCD_KEY",
	}

	groupVars := map[string]string{
		"cluster_name":       "home-kubernetes",
		"cluster_endpoint":   "192.168.1.30",
		"install_disk":       "/dev/nvme0n1",
		"installer_image":    "factory.talos.dev/metal-installer/group-schematic:v1.12.4",
		"kubernetes_version": "v1.35.2",
	}

	machineVars := map[string]string{
		"node_ip": "192.168.1.10",
	}

	data := renderer.BuildTemplateData("cp-00", machineVars, mergeSecrets(secrets, groupVars))

	result, err := engine.Render("controlplane.yaml.tmpl", renderer.RoleControlplane, data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	assertContains(t, result.Output, "merged-token")
	assertContains(t, result.Output, "hostname: cp-00")
	assertContains(t, result.Output, "192.168.1.10/16")
	assertContains(t, result.Output, "home-kubernetes")
	assertContains(t, result.Output, "group-schematic")
}

// TestRenderPipeline_ValidationFailsWithClearErrors verifies that rendering
// with incomplete data produces errors that identify the specific machine
// and all missing fields.
func TestRenderPipeline_ValidationFailsWithClearErrors(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := renderer.NewEngine(templateDir)

	// Controlplane with several missing secrets
	data := fullControlplaneData()
	data.MachineCAKey = ""
	data.EtcdCACert = ""
	data.ServiceAcctKey = ""

	_, err := engine.Render("controlplane.yaml.tmpl", renderer.RoleControlplane, data)
	if err == nil {
		t.Fatal("expected validation error")
	}

	errStr := err.Error()

	// Error must identify the hostname
	assertContains(t, errStr, "cp-00")

	// Error must mention all missing fields
	assertContains(t, errStr, "machine_ca_key")
	assertContains(t, errStr, "etcd_ca_cert")
	assertContains(t, errStr, "service_account_key")
}

// TestRenderPipeline_RenderAllMultipleMachines renders a full set of
// machines (3 CP + 3 workers) and verifies all output files.
func TestRenderPipeline_RenderAllMultipleMachines(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := renderer.NewEngine(templateDir)
	outputDir := filepath.Join(t.TempDir(), "rendered")

	specs := []renderer.RenderSpec{
		{Hostname: "cp-00", Template: "controlplane.yaml.tmpl", Role: renderer.RoleControlplane, Data: namedCP("cp-00", "192.168.1.10")},
		{Hostname: "cp-01", Template: "controlplane.yaml.tmpl", Role: renderer.RoleControlplane, Data: namedCP("cp-01", "192.168.1.11")},
		{Hostname: "cp-02", Template: "controlplane.yaml.tmpl", Role: renderer.RoleControlplane, Data: namedCP("cp-02", "192.168.1.12")},
		{Hostname: "worker-00", Template: "worker.yaml.tmpl", Role: renderer.RoleWorker, Data: namedWorker("worker-00", "192.168.1.20")},
		{Hostname: "worker-01", Template: "worker.yaml.tmpl", Role: renderer.RoleWorker, Data: namedWorker("worker-01", "192.168.1.21")},
		{Hostname: "worker-02", Template: "worker.yaml.tmpl", Role: renderer.RoleWorker, Data: namedWorker("worker-02", "192.168.1.22")},
	}

	if err := engine.RenderAll(specs, outputDir); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	// Verify all 6 files exist
	for _, spec := range specs {
		path := filepath.Join(outputDir, spec.Hostname+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		out := string(data)
		assertContains(t, out, spec.Hostname)
	}

	// Verify no stale files
	entries, _ := os.ReadDir(outputDir)
	if len(entries) != 6 {
		t.Errorf("expected 6 files, got %d", len(entries))
	}

	// Verify CP and worker files have correct types
	cpData, _ := os.ReadFile(filepath.Join(outputDir, "cp-00.yaml"))
	if !strings.Contains(string(cpData), "type: controlplane") {
		t.Error("cp-00 should be controlplane")
	}

	workerData, _ := os.ReadFile(filepath.Join(outputDir, "worker-00.yaml"))
	if !strings.Contains(string(workerData), "type: worker") {
		t.Error("worker-00 should be worker")
	}
}

// --- Helpers ---

func findTemplateDir(t *testing.T) string {
	t.Helper()
	// Walk up from the test directory to find templates/
	for _, candidate := range []string{
		"../../templates",
		"../../../templates",
		"../../../../templates",
	} {
		if _, err := os.Stat(filepath.Join(candidate, "controlplane.yaml.tmpl")); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
	}
	t.Fatal("could not find templates/ directory — run from repo root")
	return ""
}

func fullControlplaneData() *renderer.TemplateData {
	return &renderer.TemplateData{
		Hostname:        "cp-00",
		NodeIP:          "192.168.1.10",
		ClusterName:     "home-kubernetes",
		ClusterEndpoint: "192.168.1.30",
		MachineToken:    "machine-token-value",
		MachineCACert:   "machine-ca-cert-base64",
		MachineCAKey:    "machine-ca-key-base64",
		ClusterID:       "cluster-id-value",
		ClusterSecret:   "cluster-secret-value",
		ClusterToken:    "cluster-token-value",
		SecretboxKey:    "secretbox-value",
		ClusterCACert:   "cluster-ca-cert-base64",
		ClusterCAKey:    "cluster-ca-key-base64",
		AggregatorCert:  "agg-cert-base64",
		AggregatorKey:   "agg-key-base64",
		ServiceAcctKey:  "sa-key-base64",
		EtcdCACert:      "etcd-ca-cert-base64",
		EtcdCAKey:       "etcd-ca-key-base64",
		InstallDisk:     "/dev/nvme0n1",
		InstallerImage:  "factory.talos.dev/metal-installer/test-cp-schematic:v1.12.4",
	}
}

func fullWorkerData() *renderer.TemplateData {
	return &renderer.TemplateData{
		Hostname:            "worker-01",
		NodeIP:              "192.168.1.20",
		ClusterName:         "home-kubernetes",
		ClusterEndpoint:     "192.168.1.30",
		MachineToken:        "machine-token-value",
		MachineCACert:       "machine-ca-cert-base64",
		ClusterID:           "cluster-id-value",
		ClusterSecret:       "cluster-secret-value",
		ClusterToken:        "cluster-token-value",
		ClusterCACert:       "cluster-ca-cert-base64",
		InstallDiskSelector: "naa.5002538e4019fdec",
		InstallerImage:      "factory.talos.dev/metal-installer/test-worker-schematic:v1.12.4",
	}
}

func namedCP(hostname, ip string) *renderer.TemplateData {
	d := fullControlplaneData()
	d.Hostname = hostname
	d.NodeIP = ip
	return d
}

func namedWorker(hostname, ip string) *renderer.TemplateData {
	d := fullWorkerData()
	d.Hostname = hostname
	d.NodeIP = ip
	return d
}

func mergeSecrets(secrets, vars map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range secrets {
		merged[k] = v
	}
	for k, v := range vars {
		merged[k] = v
	}
	return merged
}
