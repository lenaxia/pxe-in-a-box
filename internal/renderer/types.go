// Package renderer renders Talos machine config templates (Go text/template)
// with type-safe data structs and comprehensive validation.
//
// Templates live in the config volume at /config/templates/*.tmpl and are
// rendered to /assets/rendered/<hostname>.yaml at container startup.
//
// Required variables are validated before rendering — missing values produce
// clear errors identifying which template, which variable, and which machine.
package renderer

import (
	"fmt"
	"strings"
)

// TemplateData is passed to Go templates for rendering Talos machine configs.
// Every field is explicitly typed — no map[string]string ambiguity.
type TemplateData struct {
	// Machine identity (required, unique per machine)
	Hostname string
	NodeIP   string

	// Cluster identity (required, from secrets or machines.yaml vars)
	ClusterName     string
	ClusterEndpoint string

	// Machine secrets (required for controlplane, partial for workers)
	MachineToken   string
	MachineCACert  string
	MachineCAKey   string // controlplane only
	ClusterID      string
	ClusterSecret  string
	ClusterToken   string
	SecretboxKey   string
	ClusterCACert  string
	ClusterCAKey   string // controlplane only
	AggregatorCert string // controlplane only
	AggregatorKey  string // controlplane only
	ServiceAcctKey string // controlplane only
	EtcdCACert     string // controlplane only
	EtcdCAKey      string // controlplane only

	// Install configuration (from machines.yaml vars)
	InstallDisk         string
	InstallDiskSelector string
	InstallerImage      string
	KubernetesVersion   string
	WipeDisk            bool

	// Network (optional, has defaults)
	Gateway       string
	MTU           int
	Nameservers   []string
	NodeSubnet    string
	PodSubnet     string
	ServiceSubnet string

	// Worker-specific
	IsGPU                bool
	LonghornDiskSelector string
	LonghornMinSize      string
}

// RenderRole identifies whether this is a controlplane or worker template.
type RenderRole string

const (
	RoleControlplane RenderRole = "controlplane"
	RoleWorker       RenderRole = "worker"
)

// ValidationError accumulates all missing/invalid fields so the operator
// sees every problem at once.
type ValidationError struct {
	Hostname string
	Errors   []string
}

func (ve *ValidationError) Error() string {
	if len(ve.Errors) == 0 {
		return ""
	}
	header := fmt.Sprintf("template rendering for %q failed:", ve.Hostname)
	if len(ve.Errors) == 1 {
		return fmt.Sprintf("%s\n  %s", header, ve.Errors[0])
	}
	var b strings.Builder
	b.WriteString(header)
	for i, e := range ve.Errors {
		b.WriteString(fmt.Sprintf("\n  %d. %s", i+1, e))
	}
	return b.String()
}

func (ve *ValidationError) add(field, msg string) {
	ve.Errors = append(ve.Errors, fmt.Sprintf("%s: %s", field, msg))
}

func (ve *ValidationError) hasErrors() bool {
	return len(ve.Errors) > 0
}

// Validate checks that all required fields are present for the given role.
// Returns a ValidationError listing ALL missing fields, not just the first.
func (td *TemplateData) Validate(role RenderRole) error {
	ve := &ValidationError{Hostname: td.Hostname}

	// ── Required for all machines ────────────────────────────────────
	requireStr(ve, "hostname", td.Hostname)
	requireStr(ve, "node_ip", td.NodeIP)
	requireStr(ve, "cluster_name", td.ClusterName)
	requireStr(ve, "cluster_endpoint", td.ClusterEndpoint)
	requireStr(ve, "machine_token", td.MachineToken)
	requireStr(ve, "machine_ca_cert", td.MachineCACert)
	requireStr(ve, "cluster_id", td.ClusterID)
	requireStr(ve, "cluster_secret", td.ClusterSecret)
	requireStr(ve, "cluster_token", td.ClusterToken)
	requireStr(ve, "cluster_ca_cert", td.ClusterCACert)
	requireStr(ve, "installer_image", td.InstallerImage)

	// Install target: need either disk or disk selector, not both
	if td.InstallDisk == "" && td.InstallDiskSelector == "" {
		ve.add("install", "either install_disk or install_disk_selector must be set")
	}

	// ── Required for controlplane only ───────────────────────────────
	if role == RoleControlplane {
		requireStr(ve, "machine_ca_key", td.MachineCAKey)
		requireStr(ve, "cluster_ca_key", td.ClusterCAKey)
		requireStr(ve, "aggregator_ca_cert", td.AggregatorCert)
		requireStr(ve, "aggregator_ca_key", td.AggregatorKey)
		requireStr(ve, "service_account_key", td.ServiceAcctKey)
		requireStr(ve, "etcd_ca_cert", td.EtcdCACert)
		requireStr(ve, "etcd_ca_key", td.EtcdCAKey)
		requireStr(ve, "secretbox_key", td.SecretboxKey)
	}

	if ve.hasErrors() {
		return ve
	}
	return nil
}

func requireStr(ve *ValidationError, field, value string) {
	if value == "" {
		ve.add(field, "required but not set (check machines.yaml vars and secrets.yaml)")
	}
}
