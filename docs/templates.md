# Talos Config Templates

PXE-in-a-Box uses Go `text/template` to render Talos machine configs at
container startup. This eliminates per-machine config files for machines
that share the same configuration pattern.

## Template Files

| Template | Used by | Description |
|----------|---------|-------------|
| `controlplane.yaml.tmpl` | controlplane groups | Full CP config: etcd, API server, scheduler, VIP |
| `worker.yaml.tmpl` | worker groups | Worker config: mounts, UserVolumeConfig, optional GPU |

Templates are Go `text/template` files using `{{ .FieldName }}` syntax.
If no `templates/` directory is provided in the config volume, the
defaults baked into the Docker image are used.

## What's Templated vs Static

### Shared across all machines (from secrets.yaml)

All secrets come from the secrets file — never in the repo or the image:

- Machine token, CA cert, CA key (CP only)
- Cluster ID, secret, token
- Cluster CA cert/key (CP only)
- Etcd CA cert/key (CP only)
- Aggregator CA cert/key (CP only)
- Service account key (CP only)
- Secretbox encryption secret

### Shared within a group (from group `vars`)

- `install_disk` — disk path or selector
- `installer_image` — Talos installer image with schematic hash
- `kubernetes_version` — kubelet and k8s component version
- `is_gpu` — enables NVIDIA runtime and kernel modules
- `gateway`, `mtu`, `nameservers` — network settings

### Unique per machine (from machine `vars`)

- `hostname` — from `machines.yaml`
- `node_ip` — from `machines.yaml`
- `install_disk_selector` — per-machine disk WWID
- `wipe_disk` — whether to wipe on install

## Worker Template Variants

### Standard worker (default)

```yaml
# In machines.yaml:
- mac: aa:bb:cc:dd:00:10
  hostname: worker-00
  vars:
    node_ip: 192.168.1.20
    install_disk_selector: naa.YOUR_DISK_WWID
```

Renders with: standard containerd config, longhorn + openebs mounts,
UserVolumeConfig for longhorn storage.

### GPU worker

```yaml
# In machines.yaml:
- name: workers-gpu
  profile: talos-nvidia               # IF asset with NVIDIA drivers
  template: worker.yaml.tmpl
  vars:
    is_gpu: true                       # key flag
    installer_image: factory.talos.dev/metal-installer/NVIDIA_SCHEMATIC:v1.12.4
  machines:
    - mac: aa:bb:cc:dd:00:20
      hostname: worker-gpu-00
      vars:
        node_ip: 192.168.1.30
```

Renders with: NVIDIA container runtime, nvidia kernel modules,
`bpf_jit_harden` sysctl, custom containerd config.

**Important:** GPU workers need a different `profile` (asset ID) that
references an Image Factory build with NVIDIA extensions. The stock
Talos kernel does not include NVIDIA drivers.

## When to Use Singletons Instead

Use `singletons` (direct config files, no template) when a machine has:

- A completely different schematic (custom GPU driver variant)
- Unique storage layout (SATA vs NVMe longhorn, different sizes)
- Special patches that don't fit any pattern
- Configs rendered by talhelper that you don't want to template

```yaml
singletons:
  - mac: aa:bb:cc:dd:00:30
    hostname: worker-special
    profile: talos-v1.10.6
    config: worker-special.yaml    # place in config/static/
```

The config file is served as-is via matchbox at `/assets/static/worker-special.yaml`.

## Verification

Template rendering is verified by integration tests:
- `internal/renderer/engine_test.go` — 32 unit tests for rendering + validation
- `internal/e2e/render_pipeline_integration_test.go` — renders actual shipped
  templates and verifies all sections, secrets, install config, GPU sections
