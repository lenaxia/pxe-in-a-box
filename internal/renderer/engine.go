package renderer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Engine renders Talos machine config templates using Go text/template.
type Engine struct {
	templateDir string
	cache       map[string]*template.Template
}

// NewEngine creates a renderer that loads templates from the given directory.
func NewEngine(templateDir string) *Engine {
	return &Engine{
		templateDir: templateDir,
		cache:       make(map[string]*template.Template),
	}
}

// RenderResult contains the rendered output and metadata.
type RenderResult struct {
	Hostname string
	Role     RenderRole
	Output   string
}

// Render renders a single machine's config from the template.
// The template name is the filename in the template directory (e.g.,
// "controlplane.yaml.tmpl"). The role determines which fields are required.
func (e *Engine) Render(tmplName string, role RenderRole, data *TemplateData) (*RenderResult, error) {
	if err := data.Validate(role); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	tmpl, err := e.loadTemplate(tmplName)
	if err != nil {
		return nil, fmt.Errorf("loading template %q: %w", tmplName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template %q for %q: %w", tmplName, data.Hostname, err)
	}

	return &RenderResult{
		Hostname: data.Hostname,
		Role:     role,
		Output:   buf.String(),
	}, nil
}

// RenderToFile renders a template and writes the output to a file.
func (e *Engine) RenderToFile(tmplName string, role RenderRole, data *TemplateData, outputPath string) error {
	result, err := e.Render(tmplName, role, data)
	if err != nil {
		return err
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating output directory %q: %w", dir, err)
	}

	if err := os.WriteFile(outputPath, []byte(result.Output), 0644); err != nil {
		return fmt.Errorf("writing rendered config for %q: %w", data.Hostname, err)
	}

	return nil
}

func (e *Engine) loadTemplate(name string) (*template.Template, error) {
	if cached, ok := e.cache[name]; ok {
		return cached, nil
	}

	path := filepath.Join(e.templateDir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading template file: %w", err)
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	e.cache[name] = tmpl
	return tmpl, nil
}

// RenderAll renders configs for every machine in a machines config.
// It clears the output directory first to remove stale files.
type RenderSpec struct {
	Hostname string
	Template string
	Role     RenderRole
	Data     *TemplateData
}

// RenderAll renders multiple specs and writes to files.
// If any spec fails validation, all rendering is aborted.
func (e *Engine) RenderAll(specs []RenderSpec, outputDir string) error {
	// Validate ALL specs first — don't render anything if any are broken
	for _, spec := range specs {
		if err := spec.Data.Validate(spec.Role); err != nil {
			return fmt.Errorf("validation: %w", err)
		}
	}

	// Clear output directory
	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("clearing output dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	for _, spec := range specs {
		outputPath := filepath.Join(outputDir, spec.Hostname+".yaml")
		if err := e.RenderToFile(spec.Template, spec.Role, spec.Data, outputPath); err != nil {
			return err
		}
	}

	return nil
}

// BuildTemplateData merges machine vars, group vars, and secrets into a
// TemplateData struct. Machine vars override group vars. Both override
// secrets only for non-secret fields.
func BuildTemplateData(hostname string, vars map[string]string, secrets map[string]string) *TemplateData {
	// Merge: secrets are the base, vars override
	merged := make(map[string]string)
	for k, v := range secrets {
		merged[k] = v
	}
	for k, v := range vars {
		merged[k] = v
	}
	merged["hostname"] = hostname

	td := &TemplateData{
		Hostname:             hostname,
		NodeIP:               merged["node_ip"],
		ClusterName:          merged["cluster_name"],
		ClusterEndpoint:      merged["cluster_endpoint"],
		MachineToken:         merged["machine_token"],
		MachineCACert:        merged["machine_ca_cert"],
		MachineCAKey:         merged["machine_ca_key"],
		ClusterID:            merged["cluster_id"],
		ClusterSecret:        merged["cluster_secret"],
		ClusterToken:         merged["cluster_token"],
		SecretboxKey:         merged["secretbox_key"],
		ClusterCACert:        merged["cluster_ca_cert"],
		ClusterCAKey:         merged["cluster_ca_key"],
		AggregatorCert:       merged["aggregator_ca_cert"],
		AggregatorKey:        merged["aggregator_ca_key"],
		ServiceAcctKey:       merged["service_account_key"],
		EtcdCACert:           merged["etcd_ca_cert"],
		EtcdCAKey:            merged["etcd_ca_key"],
		InstallDisk:          merged["install_disk"],
		InstallDiskSelector:  merged["install_disk_selector"],
		InstallerImage:       merged["installer_image"],
		KubernetesVersion:    merged["kubernetes_version"],
		Gateway:              merged["gateway"],
		NodeSubnet:           merged["node_subnet"],
		PodSubnet:            merged["pod_subnet"],
		ServiceSubnet:        merged["service_subnet"],
		LonghornDiskSelector: merged["longhorn_disk_selector"],
		LonghornMinSize:      merged["longhorn_min_size"],
		MTU:                  atoiOr(merged["mtu"], 0),
	}

	if merged["wipe_disk"] == "true" {
		td.WipeDisk = true
	}
	if merged["is_gpu"] == "true" {
		td.IsGPU = true
	}

	if ns := merged["nameservers"]; ns != "" {
		td.Nameservers = strings.Split(ns, ",")
		for i := range td.Nameservers {
			td.Nameservers[i] = strings.TrimSpace(td.Nameservers[i])
		}
	}

	return td
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
