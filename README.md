# PXE-in-a-Box

A self-contained PXE boot server for Talos Linux clusters. Machines boot from
network on an empty drive, self-configure, and join an existing cluster with
zero operator interaction. Unknown machines get an interactive boot menu.

Built on [matchbox](https://github.com/poseidon/matchbox) + [dnsmasq](https://dnsmasq.org/) + [iPXE](https://ipxe.org/),
packaged as a single Docker image, deployed via Ansible.

## How It Works

```
Machine PXE boots
    │
    ▼
Your router (DHCP) assigns IP, points to PXE host for TFTP
    │
    ▼
dnsmasq (proxy DHCP + TFTP) serves iPXE chainload binary
    │
    ▼
iPXE fetches custom boot.ipxe from matchbox (HTTP)
    │
    ├── MAC is known?  ──► matchbox returns kernel + initramfs + machine config URL
    │                       └── Talos boots, fetches config, installs, joins cluster
    │
    └── MAC is unknown? ──► matchbox returns 404
                              └── iPXE falls back to boot menu (Talos maintenance / Ubuntu)
```

**Key design:** No catch-all group in matchbox. Unknown MACs get HTTP 404,
which triggers iPXE's `chain || goto menu` fallback to an interactive menu.

## Features

- **Zero-touch provisioning** — known machines PXE boot and join the cluster automatically
- **Config templating** — parameterized Talos machine configs from a single `machines.yaml`
- **Boot menu** — unknown machines get an iPXE menu with configurable timeout
- **Asset management** — auto-downloads Talos kernels/initramfs (GitHub releases + Image Factory)
- **Single container** — dnsmasq + matchbox in one image, multi-arch (amd64/arm64)
- **BIOS PXE** — UEFI support planned for a future release
- **Secret safety** — gitleaks pre-commit hooks, Ansible Vault for secrets, no secrets in the image

## Quick Start

### Prerequisites

- A machine to run the PXE server (x86 or Raspberry Pi) with Docker
- A Talos cluster (existing or new) — you need the cluster secrets
- A router/DHCP server where you can configure TFTP server pointer
- Ansible on your workstation

### 1. Clone and configure

```bash
git clone https://github.com/lenaxia/pxe-in-a-box.git
cd pxe-in-a-box
```

### 2. Create your config files

Copy the examples and edit them:

```bash
mkdir -p ansible/files/config
cp examples/machines.yaml ansible/files/config/
cp examples/assets.yaml   ansible/files/config/
cp examples/menu.yaml     ansible/files/config/
```

Edit `ansible/files/config/machines.yaml` — define your machines:

```yaml
groups:
  - name: controlplane
    profile: talos-v1.10.6           # references an asset ID
    template: controlplane.yaml.j2   # Jinja2 template in templates/
    vars:
      install_disk: /dev/nvme0n1
      installer_image: factory.talos.dev/metal-installer/YOUR_SCHEMATIC:v1.12.4
    machines:
      - mac: aa:bb:cc:dd:00:01
        hostname: cp-00
        vars:
          node_ip: 192.168.1.10

  - name: workers
    profile: talos-v1.10.6
    template: worker.yaml.j2
    machines:
      - mac: aa:bb:cc:dd:00:10
        hostname: worker-00
        vars:
          node_ip: 192.168.1.20
          install_disk_selector: naa.YOUR_DISK_WWID

# Machines too unique to template get their own config file
singletons:
  - mac: aa:bb:cc:dd:00:20
    hostname: worker-gpu
    profile: talos-v1.10.6
    config: worker-gpu.yaml           # pre-rendered config in static/
```

Edit `ansible/files/config/assets.yaml` — define which OS assets to download:

```yaml
talos:
  - id: talos-v1.10.6
    version: v1.10.6
    arch: amd64
  - id: talos-nvidia
    image_factory_hash: "YOUR_SCHEMATIC_HASH"
    version: v1.9.5
    arch: amd64
    download_uki: true
```

### 3. Add your cluster secrets

Extract secrets from your existing cluster. If you use [talhelper](https://github.com/budimanjojo/talhelper):

```bash
# From your talhelper directory:
cat talsecret.sops.yaml | sops -d --extract '["machine"]'
```

Or from a rendered machine config:

```bash
# Read the token, CA cert, etc. from any existing node config
grep "token:" clusterconfig/home-kubernetes-cp-00.yaml
```

Put them in the encrypted vault file:

```bash
cd ansible
ansible-vault edit group_vars/vault.yml
# Fill in all REPLACE_WITH_* values with your real secrets
```

### 4. Configure Ansible variables

Edit `ansible/group_vars/all.yml`:

```yaml
pxe_image: ghcr.io/lenaxia/pxe-in-a-box:latest
matchbox_addr: 192.168.1.100    # PXE host IP (must be static)
matchbox_port: "8081"
dnsmasq_proxy_range: 192.168.1.1
dnsmasq_proxy_netmask: 255.255.255.0
```

Create an inventory file:

```bash
cat > ansible/inventory.ini << 'EOF'
[pxe]
192.168.1.100 ansible_user=youruser
EOF
```

### 5. Deploy

```bash
cd ansible
ansible-playbook site.yml -i inventory.ini --ask-vault-pass
```

This will:
1. Install Docker on the target host (if needed)
2. Render Talos machine configs from templates with your secrets
3. Generate matchbox groups/profiles via `pxe-gen`
4. Generate the custom `boot.ipxe` script
5. Pull the Docker image and start the container
6. The container auto-downloads kernel/initramfs assets on first start

### 6. Configure your router

Point TFTP to your PXE host IP (e.g., `192.168.1.100`). On a UniFi UDM Pro,
this is under Network Settings > Advanced > DHCP > TFTP Server.

That's it. Any machine with PXE boot enabled in BIOS will now boot based on
its MAC address.

## Configuration Reference

### `machines.yaml`

| Section | Field | Required | Description |
|---------|-------|----------|-------------|
| `groups[].` | `name` | yes | Group identifier |
| | `profile` | yes | Asset ID (from `assets.yaml`) |
| | `template` | yes | Jinja2 template filename in `templates/` |
| | `vars` | no | Shared variables for all machines in the group |
| `groups[].machines[].` | `mac` | yes | MAC address (colon-separated) |
| | `hostname` | yes | DNS-safe hostname (lowercase, hyphens) |
| | `vars` | no | Per-machine variables (override group vars) |
| `singletons[].` | `mac` | yes | MAC address |
| | `hostname` | yes | Hostname |
| | `profile` | yes | Asset ID |
| | `config` | yes | Filename in `static/` directory |

### `assets.yaml`

```yaml
cleanup: false    # opt-in: delete assets not in this manifest

talos:
  - id: talos-v1.10.6          # unique identifier, used in profile paths
    version: v1.10.6           # GitHub release tag
    arch: amd64
  - id: talos-nvidia           # Image Factory custom build
    image_factory_hash: "..."  # schematic ID from factory.talos.dev
    version: v1.9.5
    arch: amd64
    download_uki: true         # also download unified kernel image
  - id: talos-pinned           # optional checksum pinning
    version: v1.10.6
    arch: amd64
    sha256:
      vmlinuz: abc123...
      initramfs.xz: def456...

ubuntu:
  - id: ubuntu-noble
    release: noble
    arch: amd64

debian:
  - id: debian-bookworm
    release: bookworm
    arch: amd64

arch:
  - id: arch-latest
    arch: amd64
```

### `menu.yaml`

```yaml
timeout: 10            # seconds before auto-boot
default: talos         # which entry to auto-select

entries:
  - id: talos
    label: "Talos Linux (Maintenance Mode)"
    profile: talos-v1.10.6
  - id: ubuntu
    label: "Ubuntu Server 24.04"
    profile: ubuntu-noble
```

### Template Variables

The Talos config templates (`controlplane.yaml.j2`, `worker.yaml.j2`) accept
these variables from `machines.yaml` group/machine `vars`:

| Variable | Scope | Description |
|----------|-------|-------------|
| `node_ip` | per-machine | Node IP without CIDR (e.g., `192.168.1.10`) |
| `install_disk` | group | Disk path (e.g., `/dev/nvme0n1`) |
| `install_disk_selector` | per-machine | Disk WWID (alternative to `install_disk`) |
| `installer_image` | group | Talos installer image with schematic |
| `kubernetes_version` | group | Kubelet/k8s version tag (e.g., `v1.35.2`) |
| `wipe_disk` | per-machine | Wipe disk on install (default: `false`) |
| `is_gpu` | group | Enable NVIDIA runtime + kernel modules (default: `false`) |
| `nameservers` | per-machine | List of DNS servers |
| `mtu` | group | Network MTU (default: `1500`) |
| `gateway` | group | Network gateway (default: `192.168.0.1`) |

Secrets come from `vault.yml`, not `machines.yaml`.

## Architecture

For full design documentation, see [docs/architecture.md](docs/architecture.md) and
[docs/requirements.md](docs/requirements.md).

### Container internals

```
┌──────────────── Docker Container (--net host) ────────────────┐
│                                                                │
│  dnsmasq (proxy DHCP + TFTP:69)                               │
│  matchbox (HTTP:8081)                                          │
│  pxe-in-a-box (entrypoint: validate → download → serve)       │
│                                                                │
│  /config/ (mounted, read-only)                                │
│    ├── dnsmasq.conf          # proxy DHCP + TFTP config        │
│    ├── machines.yaml         # MAC → profile mapping           │
│    ├── assets.yaml           # what to download                │
│    ├── menu.yaml             # boot menu for unknown MACs      │
│    ├── groups/               # generated matchbox groups       │
│    ├── profiles/             # generated matchbox profiles     │
│    ├── templates/            # Jinja2 Talos config templates  │
│    └── static/               # one-off machine configs         │
│                                                                │
│  /assets/ (mounted, read-write)                               │
│    ├── boot.ipxe             # generated iPXE boot script      │
│    ├── rendered/             # rendered Talos machine configs  │
│    ├── static/               # singleton config files          │
│    └── talos-v1.10.6/amd64/  # downloaded kernels/initramfs    │
│                                                                │
│  /tftpboot/ (baked into image)                                │
│    ├── undionly.kpxe         # BIOS iPXE chainload             │
│    └── ipxe.efi              # UEFI iPXE chainload (not yet    │
│                               configured in dnsmasq)           │
└────────────────────────────────────────────────────────────────┘
```

## CLI Tools

### `pxe-gen` (runs on Ansible controller)

Generates matchbox groups, profiles, and boot.ipxe from config files.

```bash
pxe-gen \
  --machines machines.yaml \
  --assets assets.yaml \
  --menu menu.yaml \
  --addr 192.168.1.100 \
  --port 8081 \
  --output-dir ./generated
```

### `pxe-in-a-box` (container entrypoint)

```bash
pxe-in-a-box --config-dir /config --assets-dir /assets
pxe-in-a-box --dump-state                    # show current groups/profiles/assets
pxe-in-a-box --skip-download                 # skip asset download phase
pxe-in-a-box --cleanup                       # remove assets not in manifest
pxe-in-a-box --dry-run --cleanup             # show what would be deleted
```

## Development

### Building

```bash
make build              # compile pxe-gen and pxe-in-a-box
make docker-build       # build Docker image
make build-arm64        # cross-compile for ARM64
```

### Testing

```bash
make test-unit          # 70 unit tests (no external deps)
make test-integration   # pipeline + OS URL reachability tests
make test-e2e-http      # matchbox HTTP tests (requires matchbox binary)
make test-e2e-qemu      # full PXE boot tests (requires matchbox + QEMU)
make test-ansible       # template rendering + playbook structure tests
make test-all           # everything
```

Install test dependencies:

```bash
make download-matchbox   # install matchbox binary for e2e tests
sudo apt install qemu-system-x86  # for QEMU tests
```

### Pre-commit Hooks

```bash
pre-commit install
pre-commit install --hook-type pre-push
```

Hooks: gitleaks (secret scanning), gofmt, go vet, go mod tidy, YAML validation,
private key detection, large file blocking.

### Project Structure

```
pxe-in-a-box/
├── cmd/
│   ├── pxe-gen/           # CLI: generates matchbox configs from YAML
│   └── pxe-in-a-box/      # CLI: container entrypoint
├── internal/
│   ├── config/            # Parse + validate machines/assets/menu YAML
│   ├── matchbox/          # Generate matchbox group/profile JSON
│   ├── bootscript/        # Generate custom boot.ipxe with menu fallback
│   ├── downloader/        # Download kernels/initramfs from upstream
│   ├── cleanup/           # Remove orphaned asset directories
│   └── e2e/               # Integration tests (pipeline, OS URLs)
├── test/
│   └── e2e/               # End-to-end tests (matchbox HTTP, QEMU)
├── ansible/
│   ├── site.yml           # Deployment playbook
│   ├── group_vars/        # all.yml (vars) + vault.yml (secrets)
│   ├── templates/         # dnsmasq.conf.j2, Talos config templates
│   └── tests/             # Ansible template + structure tests
├── examples/              # Example config files
├── Dockerfile             # Multi-stage: Go build → Alpine runtime
└── Makefile               # Build, test, lint targets
```

## CI/CD

| Workflow | Trigger | What it does |
|----------|---------|-------------|
| `ci.yml` | push/PR | gitleaks, lint, unit tests, integration tests, e2e (matchbox + QEMU), multi-arch build, Docker image validation |
| `os-verify.yml` | push/PR + daily cron | Per-OS download URL reachability (Talos, Ubuntu, Debian, Arch) |
| `ansible.yml` | push/PR (ansible paths) | Template rendering tests, playbook structure tests |
| `release.yml` | tag `v*.*.*` | Multi-arch Docker push to GHCR, GitHub release |

## Network Requirements

- **DHCP**: Your existing router handles IP assignment. PXE-in-a-Box uses proxy DHCP only.
- **TFTP**: Configure your router to point TFTP to the PXE host IP.
- **Static IP**: The PXE host needs a static IP (DHCP reservation).
- **Ports**: UDP 67 (DHCP), UDP 69 (TFTP), TCP 8081 (HTTP) — all on the PXE host.
- **BIOS PXE**: Currently supports BIOS PXE clients only. UEFI is planned.

## Security

- **No secrets in the image** — all secrets in mounted volumes via Ansible Vault
- **Gitleaks pre-commit** — scans every commit for hardcoded secrets
- **Gitleaks in CI** — scans every push/PR as a gating check
- **Network segmentation recommended** — deploy on an isolated VLAN
- **Bootstrap node pattern** — apply the first control plane config out-of-band;
  subsequent nodes use join configs without the CA private key

See [docs/architecture.md](docs/architecture.md) for the full threat model.

## License

MIT

## Acknowledgments

- [matchbox](https://github.com/poseidon/matchbox) — Poseidon Labs
- [dnsmasq](https://dnsmasq.org/) — Simon Kelley
- [iPXE](https://ipxe.org/) — iPXE project
- [Talos Linux](https://www.talos.dev/) — Sidero Labs
