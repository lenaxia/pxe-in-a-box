# Talos Config Templates

PXE-in-a-Box uses Jinja2 templates to render Talos machine configs at deploy
time. This eliminates per-machine config files for machines that share the
same configuration pattern.

## Template Files

| Template | Used by | Description |
|----------|---------|-------------|
| `controlplane.yaml.j2` | controlplane group | Full CP config: etcd, API server, scheduler, VIP |
| `worker.yaml.j2` | worker groups | Worker config: mounts, UserVolumeConfig, optional GPU |

## What's Templated vs Static

### Shared across all machines (from vault.yml)

All secrets come from Ansible Vault — never in the repo or the image:

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
  profile: talos-v1.10.6
  template: worker.yaml.j2
  vars:
    is_gpu: true              # key flag
    installer_image: factory.talos.dev/metal-installer/NVIDIA_SCHEMATIC:v1.12.4
  machines:
    - mac: aa:bb:cc:dd:00:20
      hostname: worker-gpu-00
      vars:
        node_ip: 192.168.1.30
```

Renders with: NVIDIA container runtime, nvidia kernel modules,
`bpf_jit_harden` sysctl, custom containerd config.

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
    config: worker-special.yaml    # place in ansible/files/config/static/
```

The config file is served as-is via matchbox at `/assets/static/worker-special.yaml`.

## Verification

Template rendering is verified by `ansible/tests/render-check.yml`, which
renders templates with test data and asserts all key fields are present
and correct. Run with:

```bash
make test-ansible
```

For a full comparison against real talhelper-rendered configs, see the
render-check playbook. It verifies 35+ fields match the original output
including all certs, keys, network config, and cluster settings.
