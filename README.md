# PXE-in-a-Box

A self-contained PXE boot server for Talos Linux clusters. Machines boot from
network on an empty drive, self-configure, and join an existing cluster with
zero operator interaction. Unknown machines get an interactive boot menu.

Built on [matchbox](https://github.com/poseidon/matchbox) + [dnsmasq](https://dnsmasq.org/) + [iPXE](https://ipxe.org/),
packaged as a single Docker image. No Ansible, no Python, no external dependencies
beyond Docker.

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

## Quick Start

### 1. Create config directory

```bash
mkdir -p pxe/{config/templates,config/static,assets}
cd pxe
```

### 2. Copy example configs and edit

```bash
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/machines.yaml > config/machines.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/assets.yaml > config/assets.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/menu.yaml > config/menu.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/secrets.yaml > config/secrets.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/docker-compose.yml > docker-compose.yml
```

Edit `config/machines.yaml` — define your machines by MAC address.
Edit `config/secrets.yaml` — fill in your cluster secrets (extract from existing cluster).

### 3. Create dnsmasq.conf

```bash
cat > config/dnsmasq.conf << 'EOF'
dhcp-range=192.168.1.1,proxy,255.255.255.0
port=0
enable-tftp
tftp-root=/tftpboot
dhcp-userclass=set:ipxe,iPXE
pxe-service=tag:#ipxe,x86PC,"PXE chainload to iPXE",undionly.kpxe
pxe-service=tag:ipxe,x86PC,"iPXE",http://YOUR_PXE_IP:8081/assets/boot.ipxe
log-queries
log-dhcp
EOF
```

### 4. Start the container

```bash
docker compose up -d
```

The container will:
1. Load secrets from `secrets.yaml`
2. Render Talos machine configs from Go templates
3. Generate matchbox groups/profiles + boot.ipxe
4. Download kernel/initramfs assets
5. Start dnsmasq + matchbox

### 5. Configure your router

Point TFTP to your PXE host IP. On UniFi: Network Settings > Advanced > DHCP > TFTP Server.

## Configuration

### Directory Layout

```
config/                         # Mounted as /config (read-only)
├── machines.yaml               # MAC → profile + template mapping
├── assets.yaml                 # What kernels/initramfs to download
├── menu.yaml                   # Boot menu for unknown machines
├── dnsmasq.conf                # Proxy DHCP + TFTP settings
├── secrets.yaml                # Cluster secrets (or secrets.sops.yaml + age.key)
├── templates/                  # Go templates (optional, defaults baked into image)
│   ├── controlplane.yaml.tmpl
│   └── worker.yaml.tmpl
└── static/                     # One-off machine configs (for singletons)

assets/                         # Mounted as /assets (read-write)
├── boot.ipxe                   # Generated at startup
├── rendered/                   # Rendered machine configs
├── static/                     # Copied from config/static/
└── talos-v1.10.6/amd64/        # Downloaded kernels/initramfs
```

### `machines.yaml`

```yaml
groups:
  - name: controlplane
    profile: talos-v1.10.6           # asset ID from assets.yaml
    template: controlplane.yaml.tmpl # Go template
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
    template: worker.yaml.tmpl
    machines:
      - mac: aa:bb:cc:dd:00:10
        hostname: worker-00
        vars:
          node_ip: 192.168.1.20
          install_disk_selector: naa.YOUR_DISK_WWID

# Machines too unique to template
singletons:
  - mac: aa:bb:cc:dd:00:20
    hostname: worker-special
    profile: talos-v1.10.6
    config: worker-special.yaml      # file in config/static/
```

### `secrets.yaml`

Three options for managing secrets:

| Mode | Files | How it works |
|------|-------|-------------|
| **Plaintext** | `secrets.yaml` | Loaded directly (simplest, homelab on isolated VLAN) |
| **SOPS at rest** | `secrets.sops.yaml` + `age.key` | Container decrypts at startup |
| **Pre-rendered** | No secrets file | Use singletons with pre-rendered configs from talhelper |

See [docs/secrets.md](docs/secrets.md) for extraction instructions.

### Template Variables

| Variable | Scope | Description |
|----------|-------|-------------|
| `hostname` | per-machine | From machines.yaml |
| `node_ip` | per-machine | Node IP without CIDR |
| `install_disk` | group | Disk path (e.g., `/dev/nvme0n1`) |
| `install_disk_selector` | per-machine | Disk WWID |
| `installer_image` | group | Talos installer image with schematic |
| `kubernetes_version` | group | K8s version tag (default: v1.35.2) |
| `wipe_disk` | per-machine | Wipe on install (default: false) |
| `is_gpu` | group | Enable NVIDIA runtime (default: false) |
| `nameservers` | per-machine | Comma-separated DNS servers |

## CLI

```bash
# Container entrypoint (runs automatically)
pxe-in-a-box --config-dir /config --assets-dir /assets

# Debug: show current state
docker exec pxe-in-a-box pxe-in-a-box --dump-state

# Skip rendering (use pre-generated configs)
docker exec pxe-in-a-box pxe-in-a-box --skip-render

# Manual template generation
pxe-gen --config-dir /config --assets-dir /assets --addr 192.168.1.100
```

## Development

```bash
make build              # compile binaries
make test-unit          # unit tests
make test-integration   # pipeline + render tests
make test-e2e-http      # matchbox HTTP tests (needs matchbox binary)
make test-e2e-qemu      # full PXE boot tests (needs QEMU)
make docker-build       # build Docker image
```

## Project Structure

```
pxe-in-a-box/
├── cmd/
│   ├── pxe-gen/              # Renders templates, generates matchbox configs
│   └── pxe-in-a-box/         # Container entrypoint
├── internal/
│   ├── config/               # Parse + validate YAML configs
│   ├── renderer/             # Go template rendering + secrets loading
│   ├── matchbox/             # Generate matchbox group/profile JSON
│   ├── bootscript/           # Generate boot.ipxe with menu fallback
│   ├── downloader/           # Download kernels/initramfs
│   ├── cleanup/              # Remove orphaned assets
│   └── e2e/                  # Integration tests
├── test/e2e/                 # E2E tests (matchbox + QEMU)
├── templates/                # Go text/template files (baked into image)
├── examples/                 # Example configs + docker-compose
├── docs/                     # Architecture, operations, secrets guides
└── Dockerfile                # Multi-stage: Go build → Alpine runtime
```

## CI/CD

| Workflow | What |
|----------|------|
| `ci.yml` | gitleaks, lint, unit/integration/e2e tests, multi-arch build, Docker validation |
| `os-verify.yml` | Per-OS download URL reachability (daily cron) |
| `release.yml` | Multi-arch Docker push on tag |

## Requirements

- Docker on the PXE host (x86 or ARM)
- Static IP for PXE host (DHCP reservation)
- Router configured to point TFTP to PXE host
- BIOS and UEFI PXE clients supported

## License

MIT
