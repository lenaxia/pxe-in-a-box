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

### Prerequisites

- Docker on the PXE host (x86 or ARM/Raspberry Pi)
- Static IP for the PXE host (DHCP reservation on your router)
- Router/DHCP server where you can set the TFTP server address
- An existing Talos cluster (to extract secrets from)
- BIOS or UEFI PXE-capable machines

### 1. Create the directory structure

```bash
mkdir -p pxe/{config/{templates,static},assets}
cd pxe
```

### 2. Download example configs

```bash
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/machines.yaml > config/machines.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/assets.yaml > config/assets.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/menu.yaml > config/menu.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/secrets.yaml > config/secrets.yaml
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/dnsmasq.conf > config/dnsmasq.conf
curl -sL https://github.com/lenaxia/pxe-in-a-box/raw/main/examples/docker-compose.yml > docker-compose.yml
```

### 3. Edit each config file

**`config/dnsmasq.conf`** — Set your network details:
```conf
# Change to your network's subnet
dhcp-range=192.168.1.1,proxy,255.255.255.0

# Change YOUR_PXE_IP to the static IP of this host (4 places)
pxe-service=tag:ipxe,x86PC,"iPXE",http://192.168.1.100:8081/assets/boot.ipxe
pxe-service=tag:ipxe,x86-64_EFI,"iPXE",http://192.168.1.100:8081/assets/boot.ipxe
```

**`config/machines.yaml`** — Define your machines (see [machines.yaml reference](#machinesyaml-reference)).

**`config/assets.yaml`** — Define which OS assets to download (see [assets.yaml reference](#assetsyaml-reference)).

**`config/menu.yaml`** — Configure the boot menu for unknown machines.

**`config/secrets.yaml`** — Fill in your cluster secrets (see [Secrets](#secrets)).

### 4. Start the container

```bash
docker compose up -d
docker logs -f pxe-in-a-box
```

The container startup phases:
1. **Render templates** — generates Talos machine configs from `machines.yaml` + `secrets.yaml`
2. **Download assets** — fetches kernel/initramfs from GitHub or Image Factory
3. **Generate matchbox configs** — creates groups/profiles + boot.ipxe
4. **Start services** — dnsmasq (proxy DHCP + TFTP) + matchbox (HTTP)

### 5. Configure your router

Set the TFTP server address to your PXE host IP. On UniFi UDM Pro:
Network Settings > Advanced > DHCP > TFTP Server = `192.168.1.100`

### 6. Boot a machine

Set a machine to PXE boot in BIOS/UEFI. If its MAC is in `machines.yaml`,
it boots directly to its Talos profile. If unknown, it shows a boot menu.

---

## Configuration Reference

### Directory Layout

See [`examples/directory-structure.txt`](examples/directory-structure.txt) for
the complete annotated directory tree.

```
config/                         # Mounted as /config (read-write)
├── machines.yaml               # MAC → profile + template mapping
├── assets.yaml                 # What kernels/initramfs to download
├── menu.yaml                   # Boot menu for unknown machines
├── dnsmasq.conf                # Proxy DHCP + TFTP settings
├── secrets.yaml                # Cluster secrets (or secrets.sops.yaml + age.key)
├── templates/                  # Go templates (optional, defaults baked into image)
└── static/                     # One-off machine configs (for singletons)

assets/                         # Mounted as /assets (read-write, persists)
├── boot.ipxe                   # Generated at startup
├── rendered/                   # Rendered machine configs
├── static/                     # Copied from config/static/
└── talos-v1.10.6/amd64/        # Downloaded kernels/initramfs
```

### `machines.yaml` reference

```yaml
groups:
  - name: controlplane                    # group identifier
    profile: talos-v1.10.6               # asset ID from assets.yaml
    template: controlplane.yaml.tmpl     # template in templates/ dir
    vars:                                # shared by all machines in this group
      install_disk: /dev/nvme0n1
      installer_image: factory.talos.dev/metal-installer/SCHEMATIC:v1.12.4
      kubernetes_version: v1.35.2
    machines:
      - mac: aa:bb:cc:dd:00:01           # colon-separated lowercase
        hostname: cp-00                  # DNS-safe (lowercase, hyphens)
        vars:                            # per-machine overrides (merge with group vars)
          node_ip: 192.168.1.10

  - name: workers
    profile: talos-v1.10.6
    template: worker.yaml.tmpl
    vars:
      installer_image: factory.talos.dev/metal-installer/SCHEMATIC:v1.12.4
    machines:
      - mac: aa:bb:cc:dd:00:10
        hostname: worker-00
        vars:
          node_ip: 192.168.1.20
          install_disk_selector: naa.5002538e0019fdec   # disk WWID
          wipe_disk: true                                # wipe on install

  - name: workers-gpu                    # GPU workers use a different asset
    profile: talos-nvidia                # references IF asset with NVIDIA drivers
    template: worker.yaml.tmpl
    vars:
      is_gpu: true                       # enables NVIDIA runtime + kernel modules
    machines:
      - mac: aa:bb:cc:dd:00:20
        hostname: worker-gpu-00
        vars:
          node_ip: 192.168.1.30

# Singletons: machines with unique configs that don't fit a template.
# Place their pre-rendered config in config/static/.
singletons:
  - mac: aa:bb:cc:dd:00:30
    hostname: worker-special
    profile: talos-v1.10.6
    config: worker-special.yaml          # file in config/static/
```

#### Group vars vs machine vars

- **Group `vars`** apply to all machines in the group
- **Machine `vars`** override group vars on key conflict
- Both are passed to the template renderer alongside secrets
- If `template` field is set, the config is rendered from template
- If `config` field is set (singleton), the file from `static/` is served as-is

### `assets.yaml` reference

```yaml
cleanup: false                          # opt-in: delete assets not in this manifest

talos:
  # Standard release from GitHub
  - id: talos-v1.10.6                   # unique ID, used in profile paths
    version: v1.10.6                    # GitHub release tag
    arch: amd64

  # Image Factory custom build (with extensions/drivers)
  - id: talos-nvidia
    version: v1.9.5                     # Talos version to build against
    image_factory_hash: "37f5a3fbd1e1e5d2a3c4..."  # schematic ID from factory.talos.dev
    arch: amd64
    download_uki: true                  # also download metal-amd64-uki.efi

  # Pin checksums for reproducibility (optional)
  - id: talos-v1.10.6-pinned
    version: v1.10.6
    arch: amd64
    sha256:
      vmlinuz: abc123...               # expected SHA256 hex
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

#### Getting an Image Factory schematic hash

```bash
# Create a schematic with your desired extensions
curl -s https://factory.talos.dev/schematics -H "Content-Type: application/json" -d '{
  "customization": {
    "systemExtensions": {
      "officialExtensions": [
        "siderolabs/nvidia-container-toolkit-production",
        "siderolabs/nvidia-open-gpu-kernel-modules-production",
        "siderolabs/amd-ucode"
      ]
    }
  }
}' | jq .id
# Returns: "014723d767a9417ec1ba47d415ca38f4f69f6f1ae4999fabf3fbfd547c12a22e"
```

### `menu.yaml` reference

```yaml
timeout: 10                # seconds before auto-boot
default: talos             # which entry to auto-select

entries:
  - id: talos
    label: "Talos Linux (Maintenance Mode)"
    profile: talos-v1.10.6   # asset ID — kernel/initramfs to boot
  - id: ubuntu
    label: "Ubuntu Server 24.04"
    profile: ubuntu-noble
```

Unknown machines get `talos.config=null` — Talos boots in maintenance mode
(does NOT join any cluster). The operator must manually configure it.

### `dnsmasq.conf` reference

See [`examples/dnsmasq.conf`](examples/dnsmasq.conf) for a fully commented template.

Key settings:

| Setting | Purpose | Example |
|---------|---------|---------|
| `dhcp-range` | Proxy DHCP subnet | `192.168.1.1,proxy,255.255.255.0` |
| `port=0` | Disable DNS | Always `0` |
| `enable-tftp` | Enable TFTP server | Always present |
| `tftp-root` | TFTP file root | `/tftpboot` (baked into image) |
| `dhcp-userclass` | Detect iPXE | `set:ipxe,iPXE` |
| `pxe-service` (stage 1) | BIOS → `undionly.kpxe`, UEFI → `ipxe.efi` | Per-architecture |
| `pxe-service` (stage 2) | iPXE → HTTP boot script URL | `http://IP:8081/assets/boot.ipxe` |

---

## Secrets

### Three modes (auto-detected)

| Mode | Files in config/ | When to use |
|------|-------------------|-------------|
| **Plaintext** | `secrets.yaml` | Simplest. Homelab on isolated VLAN. |
| **SOPS at rest** | `secrets.sops.yaml` + `age.key` | Secrets encrypted at rest. Container decrypts at startup. |
| **Pre-rendered** | (no secrets file) | Max security. Use singletons with configs from `talhelper genconfig`. |

### `secrets.yaml` fields

All values are base64-encoded PEM certificates/keys or token strings.
Extract from your existing cluster using `talhelper`, `talosctl`, or
by reading existing machine configs. See [docs/secrets.md](docs/secrets.md).

| Field | Required for | Description |
|-------|-------------|-------------|
| `machine_token` | All machines | Machine join token (durable, reusable) |
| `machine_ca_cert` | All machines | Machine CA certificate (base64 PEM) |
| `machine_ca_key` | Controlplane only | Machine CA private key (base64 PEM) |
| `cluster_id` | All machines | Cluster ID |
| `cluster_secret` | All machines | Cluster secret |
| `cluster_name` | All machines | Cluster name (e.g., `home-kubernetes`) |
| `cluster_endpoint` | All machines | Control plane VIP or endpoint IP |
| `cluster_token` | All machines | Cluster bootstrap token |
| `secretbox_key` | Controlplane only | Secretbox encryption secret |
| `cluster_ca_cert` | All machines | Cluster CA certificate (base64 PEM) |
| `cluster_ca_key` | Controlplane only | Cluster CA private key (base64 PEM) |
| `aggregator_ca_cert` | Controlplane only | Aggregator CA certificate |
| `aggregator_ca_key` | Controlplane only | Aggregator CA private key |
| `service_account_key` | Controlplane only | Service account RSA key |
| `etcd_ca_cert` | Controlplane only | Etcd CA certificate |
| `etcd_ca_key` | Controlplane only | Etcd CA private key |

### Template variables (from machines.yaml)

These come from `machines.yaml` group/machine `vars`, not secrets:

| Variable | Scope | Description |
|----------|-------|-------------|
| `hostname` | per-machine | Set automatically from machines.yaml |
| `node_ip` | per-machine | Node IP without CIDR (e.g., `192.168.1.10`) |
| `install_disk` | group | Disk path (e.g., `/dev/nvme0n1`) |
| `install_disk_selector` | per-machine | Disk WWID (alternative to install_disk) |
| `installer_image` | group | Talos installer image with schematic hash |
| `kubernetes_version` | group | K8s version tag (default: `v1.35.2`) |
| `wipe_disk` | per-machine | Wipe on install (default: `false`) |
| `is_gpu` | group | Enable NVIDIA runtime + kernel modules (default: `false`) |
| `nameservers` | per-machine | Comma-separated DNS servers |
| `gateway` | group | Network gateway (default: `192.168.0.1`) |
| `mtu` | group | Network MTU (default: `1500`) |

---

## CLI

```bash
# Container entrypoint (runs automatically on `docker compose up`)
pxe-in-a-box --config-dir /config --assets-dir /assets

# Useful flags:
pxe-in-a-box --dump-state              # show current groups/profiles/assets
pxe-in-a-box --skip-render             # skip template rendering
pxe-in-a-box --skip-download           # skip asset download
pxe-in-a-box --cleanup                 # remove assets not in manifest
pxe-in-a-box --dry-run --cleanup       # preview what would be deleted

# Manual generation (for debugging, outside container):
pxe-gen --config-dir /config --assets-dir /assets --addr 192.168.1.100 --port 8081
```

---

## Troubleshooting

### Machine loops at PXE, never gets HTTP script

dnsmasq may not be detecting iPXE user-class. Check logs:
```bash
docker logs pxe-in-a-box 2>&1 | grep dnsmasq
```
Look for repeated TFTP transfers (same file downloaded multiple times).
This means stage 1 is looping — the iPXE binary isn't reporting its user-class.

### Known MAC gets 404 instead of boot script

```bash
# Check if the group exists
docker exec pxe-in-a-box ls /config/groups/

# Check matchbox directly
curl http://PXE_IP:8081/ipxe?mac=YOUR:MAC:HERE
```
Matchbox normalizes MACs to lowercase colon-separated. Ensure your
`machines.yaml` uses the same format.

### Talos boots but can't fetch machine config

```bash
# Verify the config URL is reachable
curl http://PXE_IP:8081/assets/rendered/HOSTNAME.yaml
# For singletons:
curl http://PXE_IP:8081/assets/static/HOSTNAME.yaml
```

### Kernel/initramfs download fails

```bash
# Check container logs for download errors
docker logs pxe-in-a-box 2>&1 | grep warn

# Verify assets exist
docker exec pxe-in-a-box ls /assets/talos-*/amd64/
```

### NVIDIA modules not found

You're serving the stock Talos kernel to a GPU machine. Create an Image
Factory asset with NVIDIA extensions and reference it in the machine's
`profile` field. See [assets.yaml reference](#assetsyaml-reference).

### Machine boots to maintenance mode instead of joining

Either:
1. The MAC isn't in `machines.yaml` (unknown → menu → maintenance)
2. The config has `talos.config=null` (check the profile in `/config/profiles/`)

### Debug with dump-state

```bash
docker exec pxe-in-a-box pxe-in-a-box --dump-state
```

Shows all groups, profiles, and assets currently loaded.

---

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
