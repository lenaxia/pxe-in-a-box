# PXE-in-a-Box — Final Requirements

---

## 0. Context & Design Principles

A self-contained network boot service for a homelab Talos cluster. Machines boot from network on an empty drive, self-configure, and join an existing cluster with zero operator interaction. Unknown machines get a boot menu. Built on matchbox + dnsmasq + iPXE, packaged as a Docker image, deployed via Ansible.

**Design principles:**
- **Don't reinvent matchbox.** The system generates matchbox-native configs (groups, profiles) from a higher-level operator-facing config. Matchbox handles MAC matching and file serving at runtime.
- **Don't reinvent Talos.** The system does not manage cluster lifecycle, upgrades, or node membership. It provides the boot/provisioning path only.
- **Never generate secrets.** The system is a pure consumer of operator-provided secrets. It supports existing clusters (extract and reuse certs) and new clusters (operator runs `talosctl gen secret` manually).
- **Config is the product.** The innovation is the config layer (`machines.yaml` → templates → matchbox groups/profiles). The PXE plumbing is stable and solved.
- **Ansible is the reload mechanism.** No hot reload. Config changes = re-run playbook. Predictable, auditable, GitOps-native.

---

## 1. Core System Behavior

### 1.1 Boot Flow — Known Machines (Zero-Touch)

A machine whose MAC address is defined in `machines.yaml` boots directly to its assigned profile with no menu, no timeout, no interaction.

**REQ-1.1.1**: Known machines must boot directly — no menu, no timeout, no user interaction.

**REQ-1.1.2**: The boot must be fully automated from PXE to cluster join on an empty disk: PXE → iPXE → kernel + initrd → fetch Talos machine config → install to disk → join cluster.

**REQ-1.1.3**: The cycle must be idempotent — a machine taken offline and brought back must re-boot and rejoin without issues. Talos itself handles the "already installed" case (boots from disk rather than reinstalling).

**REQ-1.1.4**: Kernel boot arguments must include `talos.config=<url>` pointing to the rendered machine config served by matchbox over HTTP. Each machine gets its own `talos.config` URL pointing to its own rendered config file.

**Full boot sequence:**
1. Machine PXE boots → UDM Pro DHCP server assigns IP and tells client TFTP server is at 192.168.2.103
2. dnsmasq proxy DHCP responds with TFTP iPXE binary (`undionly.kpxe` for BIOS, `ipxe.efi` for UEFI)
3. Machine loads iPXE via TFTP → iPXE fetches `http://192.168.2.103:8081/assets/boot.ipxe` over HTTP
4. Matchbox matches MAC against generated groups → serves iPXE script with kernel, initrd, and kernel args (including per-machine `talos.config` URL)
5. iPXE loads kernel + initrd
6. Talos boots → fetches `talos.config` URL → reads machine config → installs to disk → reboots
7. Machine boots from disk → Talos running → joins cluster

### 1.2 Boot Flow — Unknown Machines (Menu)

A machine whose MAC is not in `machines.yaml` is presented with an iPXE boot menu.

**REQ-1.2.1**: Unknown machines receive an iPXE interactive boot menu with at minimum: Talos Linux (default) and Ubuntu Server.

**REQ-1.2.2**: Menu timeout is configurable (default: 10 seconds). On timeout, auto-boots Talos.

**REQ-1.2.3**: Menu entries are defined in `menu.yaml` — no code changes required to add/remove options.

**REQ-1.2.4**: Talos fallback for unknown machines must boot into **maintenance mode** (`talos.config=null` or a minimal config with no cluster join). The machine boots Talos but does not join any cluster. Operator must manually intervene to configure it. Unknown machines must never auto-join a cluster.

**REQ-1.2.5**: The catch-all matchbox group must serve a custom iPXE script (not a standard kernel/initrd profile) that implements an interactive boot menu with timeout. This script uses iPXE's native `:menu`, `item`, `choose`, and `goto` commands. Matchbox's native profile format does not support menus — this is a custom iPXE script generated from `menu.yaml` at deploy time and served as a static file by matchbox.

**REQ-1.2.6**: The menu iPXE script must chain to the selected OS's kernel/initrd (served by matchbox as assets) on selection or timeout. Each menu entry references a profile (kernel/initrd/args) by ID.

**Example `menu.yaml`:**
```yaml
timeout: 10
default: talos-maintenance
entries:
  - id: talos-maintenance
    label: "Talos Linux (Maintenance Mode)"
    profile: talos-maintenance
  - id: ubuntu-noble
    label: "Ubuntu Server 24.04"
    profile: ubuntu-noble
```

### 1.3 Proxy DHCP + TFTP

**REQ-1.3.1**: dnsmasq operates in proxy DHCP mode only. No IP leases. The UDM Pro (main DHCP server) handles IP assignment. dnsmasq only supplements DHCP responses with PXE boot information.

**REQ-1.3.2**: TFTP serves iPXE chainload binaries (`undionly.kpxe` for BIOS, `ipxe.efi` for UEFI).

**REQ-1.3.3**: Two-stage chainload: PXE client → TFTP (iPXE binary) → HTTP (matchbox iPXE script).

**REQ-1.3.4**: dnsmasq DNS is disabled (`port=0`).

**REQ-1.3.5**: DHCP proxy range, subnet, and matchbox endpoint are configurable via `dnsmasq.conf`.

### 1.4 Matchbox HTTP Server

**REQ-1.4.1**: Matchbox serves dynamically generated iPXE boot scripts based on MAC address (via groups + profiles).

**REQ-1.4.2**: Matchbox serves kernel, initramfs, rendered machine config files, and the custom iPXE menu script over HTTP.

**REQ-1.4.3**: Matchbox groups and profiles are generated from operator config at deploy time (see Section 2). Matchbox's native matching logic (MAC selector → profile → iPXE script) is used. No custom matching logic is implemented.

**REQ-1.4.4**: A catch-all group (no MAC selector) is generated last, pointing to the custom iPXE menu script for unknown machines.

---

## 2. Configuration Model

### 2.1 Machine Definitions (`machines.yaml`)

**REQ-2.1.1**: A single `machines.yaml` file maps machines to boot profiles and machine configs. Schema:

```yaml
# Groups of machines sharing a parameterized template
groups:
  - name: controlplane
    profile: talos-v1.10.6           # references an asset ID / auto-generated profile
    template: controlplane.yaml.j2    # Jinja2 template in /config/templates/
    vars:                             # shared across all machines in this group
      cluster_name: homelab
      cluster_endpoint: 192.168.1.10
      kubernetes_version: v1.32.0
    machines:
      - mac: 00:e0:4c:68:00:8e
        hostname: cp00
        vars:                         # per-machine overrides merge with group vars
          node_ip: 192.168.1.11
      - mac: f4:4d:30:68:a3:b3
        hostname: cp01
        vars:
          node_ip: 192.168.1.12

  - name: workers
    profile: talos-v1.10.6
    template: worker.yaml.j2
    vars:
      cluster_name: homelab
      cluster_endpoint: 192.168.1.10
    machines:
      - mac: 00:e0:4c:68:00:a1
        hostname: worker01
      - mac: 00:e0:4c:68:00:0e
        hostname: worker02

# One-off machines with a direct config file (no template)
singletons:
  - mac: b8:ae:ed:73:c3:bc
    hostname: melfina
    profile: talos-v1.9.1
    config: melfina.yaml              # direct file in /config/static/
```

**REQ-2.1.2**: Each machine maps to exactly one profile (via group `profile` or singleton `profile`). A profile defines the boot config (kernel, initrd, kernel args).

**REQ-2.1.3**: Group machines share a template and a set of shared variables. Per-machine variables merge with (and override) group variables.

**REQ-2.1.4**: Singletons reference a direct config file — no template rendering.

**REQ-2.1.5**: Duplicate MACs across groups or singletons must be rejected at validation time. Fail fast with a clear error identifying both entries.

**REQ-2.1.6**: MAC matching priority: specific MAC (group or singleton) → catch-all (menu). Matchbox's native priority handles this — generated groups are ordered with specific MAC groups first, catch-all last.

### 2.2 Template Rendering

**REQ-2.2.1**: Templates use Jinja2 (consistent with Ansible ecosystem).

**REQ-2.2.2**: Templates live in `/config/templates/`.

**REQ-2.2.3**: At deploy time, Ansible renders each template with merged group + machine variables. Variables include secrets decrypted from Ansible Vault.

**REQ-2.2.4**: Rendered configs are written to `/config/rendered/<hostname>.yaml`.

**REQ-2.2.5**: Rendered configs are served by matchbox as static assets. The per-machine profile's kernel arg `talos.config=http://<matchbox>:<port>/rendered/<hostname>.yaml` is set automatically during profile generation.

**REQ-2.2.6**: Template variables must be documented per template (inline comments or a companion README).

**REQ-2.2.7**: Template rendering errors must produce a clear error: which template, which variable, what failed. No partial output — if one template fails, all rendering fails and the deploy aborts.

**REQ-2.2.8**: The rendering process must clear the `/config/rendered/` directory before each render cycle. Stale rendered configs from removed machines must not persist.

### 2.3 Profile Definitions & Generation

**REQ-2.3.1**: Profiles are matchbox-native JSON files in `/config/profiles/`.

**REQ-2.3.2**: Each profile references asset paths (kernel, initrd) by the asset `id` from the asset manifest (e.g., `/assets/talos-v1.10.6/amd64/vmlinuz`).

**REQ-2.3.3**: Profiles include kernel args: `talos.platform=metal`, `console=tty0`, `talos.config=<url>`, and security args (`init_on_alloc=1`, `slab_nomerge`, `pti=on`).

**REQ-2.3.4**: The `talos.config` URL in each profile is constructed automatically from the matchbox endpoint + the rendered config path for that machine. Operators do not set this manually.

**REQ-2.3.5**: The maintenance/fallback profile (for unknown machines via the menu) uses `talos.config=null` — no cluster join, maintenance mode only.

**REQ-2.3.6**: The system auto-generates default Talos profiles for each Talos entry in the asset manifest. A default profile includes standard kernel args (`talos.platform=metal`, console, security args) and references the asset's kernel/initrd paths. This reduces boilerplate — operators don't hand-write profiles for standard Talos boots. Manual profiles are still supported for custom kernel args or non-standard configurations.

**REQ-2.3.7**: Profiles may reference a UKI (unified kernel image) file instead of separate kernel+initrd for UEFI boot. The iPXE script uses `kernel <uki.efi>` instead of `kernel <vmlinuz>` + `initrd <initramfs>`. This is primarily for Image Factory assets that provide a UKI.

**REQ-2.3.8**: Profiles are generated per-machine (not per-group). Each machine's profile includes the correct `talos.config` URL pointing to its own rendered config file. The kernel/initrd paths are shared across machines using the same Talos version, but the `talos.config` arg is unique per machine. Profiles are auto-generated at deploy time — not hand-written for each machine. The profile `id` follows the convention `<asset-id>-<hostname>` (e.g., `talos-v1.10.6-worker01`).

### 2.4 Matchbox Group Generation

**REQ-2.4.1**: At deploy time, `machines.yaml` is processed to generate matchbox-native group JSON files in `/config/groups/`.

**REQ-2.4.2**: Each machine entry becomes a matchbox group with a MAC selector pointing to its auto-generated per-machine profile.

**REQ-2.4.3**: A catch-all group (no selector) is generated last, pointing to the custom iPXE menu script for unknown machines.

**REQ-2.4.4**: The generated groups are what matchbox reads at runtime. No custom matching logic.

**REQ-2.4.5**: The group generation process must clear the `/config/groups/` directory before each generation cycle. Stale group JSON files from removed machines must not persist.

**REQ-2.4.6**: Each machine in `machines.yaml` generates its own matchbox group JSON (with MAC selector) pointing to its own auto-generated profile JSON. This ensures each machine gets its unique `talos.config` URL in the kernel args.

### 2.5 Config Validation

**REQ-2.5.1**: At container startup (before services start), validate:
- `machines.yaml` schema (required fields, valid MAC addresses, no duplicate MACs)
- `assets.yaml` schema (valid entries, no duplicate IDs, required fields per OS type)
- `menu.yaml` schema (valid entries, referenced profiles exist)
- All referenced profiles exist
- All referenced templates exist
- All referenced assets exist on disk (or will be downloaded)
- All referenced static config files (for singletons) exist

**REQ-2.5.2**: Invalid config must fail fast with a clear error message. No services start until config is valid.

### 2.6 Config Reload

**REQ-2.6.1**: There is no hot reload. Configuration changes require an Ansible re-deploy, which re-renders templates, regenerates matchbox groups and profiles, and restarts the container. This is intentional — predictable, auditable, GitOps-native. The operator edits config in Git, runs the playbook, and the system converges.

**REQ-2.6.2**: Container restart causes a brief PXE service interruption (seconds). This is acceptable for a homelab. Not suitable for environments requiring zero-downtime config updates.

---

## 3. Asset Management

### 3.1 Asset Manifest (`assets.yaml`)

**REQ-3.1.1**: A single `assets.yaml` defines all OS assets the system manages. Example:

```yaml
cleanup: false    # opt-in cleanup (see REQ-3.4.2)

talos:
  - id: talos-v1.10.6
    version: v1.10.6
    arch: amd64
  - id: talos-v1.9.1
    version: v1.9.1
    arch: amd64
  - id: talos-nvidia
    image_factory_hash: "37f5a3fbd1e1e5d2a3c4..."
    version: v1.9.5
    arch: amd64
    download_uki: true              # also download metal-amd64-uki.efi

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

**REQ-3.1.2**: Each entry has a unique `id` that maps to the asset directory name (e.g., `/assets/talos-v1.10.6/amd64/`).

**REQ-3.1.3**: Entries with `version` download official release artifacts from upstream sources. Entries with `image_factory_hash` download from the Sidero Image Factory API for custom Talos images (kernel + initramfs with extensions baked in).

**REQ-3.1.4**: An empty or missing manifest means no assets are managed (system runs with whatever is manually on disk, but cleanup and download do not run).

**REQ-3.1.5**: Optional checksum pinning for reproducibility or air-gapped verification:
```yaml
talos:
  - id: talos-v1.10.6
    version: v1.10.6
    arch: amd64
    sha256:
      vmlinuz: abc123...
      initramfs.xz: def456...
```

### 3.2 Automated Download

**REQ-3.2.1**: At container startup, read the asset manifest and download any missing assets to the mounted assets volume. Downloads run before dnsmasq and matchbox start.

**REQ-3.2.2**: Talos official releases: download `vmlinuz` and `initramfs.xz` from `https://github.com/siderolabs/talos/releases/download/<version>/`.

**REQ-3.2.3**: Talos Image Factory builds: download `vmlinuz` and `initramfs.xz` using the schematic hash and version from the Image Factory API. Optionally download `metal-amd64-uki.efi` if `download_uki: true`.

**REQ-3.2.4**: Ubuntu: download netboot artifacts (`linux`, `initrd`, `pxelinux.0`, `ldlinux.c32`, `bootx64.efi`, `grubx64.efi`) from the Ubuntu netboot repository.

**REQ-3.2.5**: Debian: download `linux` and `initrd.gz` from the Debian netboot repository.

**REQ-3.2.6**: Arch: download `vmlinuz-linux` and `initramfs-linux.img` from an Arch mirror.

**REQ-3.2.7**: Downloads are idempotent — existing files that pass a size check are not re-downloaded.

**REQ-3.2.8**: Failed downloads retry up to 3 times with backoff. After max retries, the failure is logged as a **warning** (not an error). The container continues to start. Other assets and services are not blocked by a single asset's download failure.

**REQ-3.2.9**: The container must only fail to start if zero assets are available (i.e., the assets directory is completely empty and no downloads succeeded). If at least one asset is available, services start.

**REQ-3.2.10**: A startup summary must be logged listing: which assets are available, which assets failed to download, and which profiles/machines are affected by missing assets.

**REQ-3.2.11**: Download progress is logged to stdout.

### 3.3 Integrity Verification

**REQ-3.3.1**: Where upstream provides checksums (Talos releases publish SHA256), verify downloaded files against them.

**REQ-3.3.2**: If checksums are pinned in the manifest (REQ-3.1.5), verify against the pinned values.

**REQ-3.3.3**: Failed verification deletes the corrupt file, logs the error, and retries download (up to max retries). If still failing after retries, the asset is treated as missing (warning, not fatal — per REQ-3.2.8).

### 3.4 Automatic Cleanup

**REQ-3.4.1**: After downloads complete, compare the assets directory against the manifest. Identify any asset `id` directory not in the manifest.

**REQ-3.4.2**: Cleanup is **opt-in**. Enable via `cleanup: true` in `assets.yaml` or a `--cleanup` CLI flag. Default is **off** to prevent accidental data loss from manifest typos.

**REQ-3.4.3**: When enabled, cleanup deletes asset directories not in the manifest and logs every deleted file/directory for auditability.

**REQ-3.4.4**: A `--dry-run` cleanup mode logs what would be deleted without actually deleting.

### 3.5 Asset Path Convention

**REQ-3.5.1**: Assets stored under `/assets/<id>/<arch>/` with consistent filenames per OS type:
```
/assets/talos-v1.10.6/amd64/vmlinuz
/assets/talos-v1.10.6/amd64/initramfs.xz
/assets/talos-nvidia/amd64/vmlinuz
/assets/talos-nvidia/amd64/initramfs.xz
/assets/talos-nvidia/amd64/metal-amd64-uki.efi     # if downloaded
/assets/ubuntu-noble/amd64/linux
/assets/ubuntu-noble/amd64/initrd
/assets/ubuntu-noble/amd64/pxelinux.0
/assets/ubuntu-noble/amd64/ldlinux.c32
/assets/ubuntu-noble/amd64/bootx64.efi
/assets/ubuntu-noble/amd64/grubx64.efi
/assets/debian-bookworm/amd64/linux
/assets/debian-bookworm/amd64/initrd.gz
/assets/arch-latest/amd64/vmlinuz-linux
/assets/arch-latest/amd64/initramfs-linux.img
```

### 3.6 iPXE Binaries

**REQ-3.6.1**: iPXE chainload binaries (`undionly.kpxe`, `ipxe.efi`) are baked into the Docker image. Small, stable, required before HTTP is available. Not managed by the asset manifest.

**REQ-3.6.2**: Served via TFTP from `/tftpboot/` inside the container.

---

## 4. Security Model

### 4.1 Threat Model

This is a homelab on a private network. The system serves Talos machine configs over HTTP, which may contain cluster CA private key (bootstrap only), join tokens, and etcd credentials.

| Threat | Likelihood | Mitigation |
|--------|-----------|------------|
| Someone on main network curls matchbox | Low | Network segmentation (primary control) |
| Passive sniffing of PXE boot | Low | Matchbox TLS (optional, secondary) |
| Secret leakage via Git | Medium | Ansible Vault |
| Secret on disk on matchbox host | Accepted | Segmented network, restricted host access |

### 4.2 Network Segmentation (Primary Control)

**REQ-4.2.1**: The system must be deployed on an isolated VLAN/subnet. Only the PXE server and booting machines are on it.

**REQ-4.2.2**: This is documented as the primary security control. The system does not provide its own network isolation — it relies on the operator's network configuration.

### 4.3 Secret Management

**REQ-4.3.1**: Talos CA key, CA cert, join token, etcd CA, and other secrets are stored encrypted in Ansible Vault (or SOPS).

**REQ-4.3.2**: Ansible decrypts at deploy time and renders into machine config templates as variables.

**REQ-4.3.3**: Rendered configs (plaintext) exist only on the matchbox volume on the target host — not in Git, not in the Docker image.

**REQ-4.3.4**: Join tokens are durable and reusable — no per-boot rotation needed. Multiple machines can use the same token simultaneously or at different times.

**REQ-4.3.5**: The system must **never generate Talos secrets**. All secrets are operator-provided via Ansible Vault. This explicitly supports two scenarios:
- **Existing cluster**: Extract certs/token from the running cluster (`talosctl read` or from existing machine configs), store in Vault, render into templates. New nodes join the existing cluster.
- **New cluster**: Operator runs `talosctl gen secret` manually, stores results in Vault, then deploys.

The system has no `talosctl gen secret` task, no secret generation logic, no cert creation. It is a pure consumer of secrets.

### 4.4 Bootstrap Node Secret Handling

**REQ-4.4.1**: The first control plane node's config (containing the CA private key) should be applied out-of-band — not served via matchbox HTTP. Options: USB stick, `talosctl apply-config --insecure --nodes <ip> -f config.yaml`, or direct console access.

**REQ-4.4.2**: Subsequent control plane nodes and all workers use join configs that contain only the CA cert (public), join token, and control plane endpoint — not the CA private key.

**REQ-4.4.3**: This limits CA key exposure to one manual step. All other configs served via matchbox contain only low-value secrets.

**REQ-4.4.4**: This is a **recommendation and documented operational practice**, not enforced by the system. Templates are just templates — what's in them is the operator's choice. The operator can serve all configs via matchbox if they accept the risk on a segmented network. The README must document this guidance clearly.

### 4.5 Matchbox TLS (Optional, Defense in Depth)

**REQ-4.5.1**: Matchbox supports TLS (`-cert-file`, `-key-file`). The system must support enabling this via config.

**REQ-4.5.2**: When TLS is enabled, kernel args use `https://` for `talos.config` and iPXE script URLs.

**REQ-4.5.3**: iPXE must trust the matchbox cert — either a custom iPXE build with the CA embedded, or a cert from a CA the iPXE build already trusts.

**REQ-4.5.4**: This is a secondary control. The primary control is network segmentation.

### 4.6 No Authentication

**REQ-4.6.1**: The system does NOT provide authentication on matchbox HTTP endpoints. Matchbox has no auth. Security is handled by network segmentation and the bootstrap node pattern.

---

## 5. Deployment

### 5.1 Docker Image

**REQ-5.1.1**: A single Docker image contains dnsmasq, matchbox, and an asset downloader script. A process supervisor (s6 or supervisord) manages dnsmasq and matchbox within the container.

**REQ-5.1.2**: The image is multi-arch (amd64, arm64) for deployment on x86 hosts or Raspberry Pi.

**REQ-5.1.3**: Base image should be minimal (Alpine or Debian slim). Target image size under 200MB (excluding mounted assets).

**REQ-5.1.4**: iPXE binaries are baked in. All config, templates, and assets are mounted volumes.

**REQ-5.1.5**: The container entrypoint executes in phases:
1. Validate config (`machines.yaml`, `assets.yaml`, `menu.yaml`, referenced files)
2. Download missing assets (per manifest)
3. Optional cleanup (if enabled)
4. Log startup summary (available assets, missing assets, affected machines)
5. Start dnsmasq and matchbox via process supervisor

**REQ-5.1.6**: Images must be version-tagged (not just `latest`).

### 5.2 Ansible Deployment

**REQ-5.2.1**: An Ansible playbook deploys the container to a target host (Pi, NUC, or any Linux machine).

**REQ-5.2.2**: The playbook: installs Docker if needed, renders templates from Jinja2 with Vault-decrypted secrets, generates matchbox groups and profiles, copies all config to the target, pulls the image, and starts/restarts the container.

**REQ-5.2.3**: The playbook is idempotent — running it multiple times converges to the desired state.

**REQ-5.2.4**: Config changes require re-running the playbook. This is the reload mechanism.

**REQ-5.2.5**: The playbook should support deploying to multiple hosts (e.g., primary PXE server + cold standby) via inventory groups.

**REQ-5.2.6**: The playbook must **pre-validate** `machines.yaml`, `assets.yaml`, and `menu.yaml` locally (before copying to the target). If validation fails, the playbook aborts before touching the target's config. This prevents broken config from reaching the running PXE server. The container's own validation at startup is the second line of defense.

**REQ-5.2.7**: The inventory structure and required variables must be documented. At minimum: target host, matchbox endpoint IP, network interface, Vault password file path.

**REQ-5.2.8**: First-time setup documentation must cover extracting secrets from an existing cluster (or running `talosctl gen secret` for a new cluster) and storing them in Ansible Vault. This is a documented manual step, not an automated task.

### 5.3 Kubernetes Deployment (Secondary)

**REQ-5.3.1**: K8s manifests provided as a bonus, not the primary target.

**REQ-5.3.2**: Uses `hostNetwork: true`, single replica, node affinity.

**REQ-5.3.3**: Init container for asset download + config validation. Main container runs dnsmasq + matchbox.

**REQ-5.3.4**: Config via ConfigMap, assets via PVC.

### 5.4 Directory Structure

**REQ-5.4.1**: The system expects this mounted volume layout:

```
/config/
  dnsmasq.conf                  # DHCP proxy + TFTP settings
  assets.yaml                   # Asset manifest (what to download/cleanup)
  machines.yaml                 # MAC → profile + template + variables
  menu.yaml                     # Boot menu for unknown machines
  templates/                    # Jinja2 templates (*.j2)
  profiles/                     # Generated boot profiles (JSON, matchbox-native)
  rendered/                     # Rendered machine configs (generated at deploy)
  static/                       # One-off configs (referenced by singletons)
  groups/                       # Generated matchbox groups (generated at deploy)
/assets/                        # Downloaded kernels, initramfs (managed by assets.yaml)
/tftpboot/                      # iPXE binaries (baked into image)
```

**REQ-5.4.2**: `/config/profiles/`, `/config/groups/`, and `/config/rendered/` are generated at deploy time. The directories are cleared before each generation cycle.

### 5.5 Raspberry Pi (ARM64)

**REQ-5.5.1**: The image must build and run on ARM64.

**REQ-5.5.2**: All internal binaries (dnsmasq, matchbox, process supervisor) must have ARM64 builds available.

**REQ-5.5.3**: Resource usage suitable for Pi 4/5 (target: < 512MB RAM, minimal CPU at idle).

**REQ-5.5.4**: The PXE clients served are x86_64 (BIOS and UEFI). ARM64 PXE client support is not in scope.

---

## 6. Operational

### 6.1 Logging

**REQ-6.1.1**: All process logs (dnsmasq, matchbox, downloader, config validator) output to stdout/stderr.

**REQ-6.1.2**: Log level is configurable via environment variable.

**REQ-6.1.3**: Boot events are logged: MAC address, matched profile, config served, timestamp.

**REQ-6.1.4**: A `--dump-state` entrypoint flag prints all groups, profiles, rendered config paths, and their associated MACs/hostnames. Useful for debugging without inspecting individual JSON files.

**REQ-6.1.5**: Matchbox's HTTP endpoint should serve a directory listing or index for `/rendered/` and `/assets/` so operators can verify available files via browser/curl.

### 6.2 Health Checks

**REQ-6.2.1**: A health check validates: dnsmasq TFTP port is listening, matchbox HTTP port is responding with 200.

**REQ-6.2.2**: Docker `HEALTHCHECK` and K8s `livenessProbe`/`readinessProbe` definitions provided.

### 6.3 Statelessness

**REQ-6.3.1**: The container is stateless. All persistent state (configs, templates, assets, rendered configs) is in mounted volumes.

**REQ-6.3.2**: Destroying and recreating the container with the same mounted config produces identical behavior.

### 6.4 Reproducibility

**REQ-6.4.1**: Same image + same config + same manifest = identical behavior and asset set.

**REQ-6.4.2**: Images are version-tagged.

**REQ-6.4.3**: Config validation at startup — invalid configs fail fast before services start.

---

## 7. Non-Requirements (Out of Scope)

| # | Item | Rationale |
|---|------|-----------|
| NR-1 | **DHCP IP lease distribution** | Proxy DHCP only. UDM Pro handles IP assignment. |
| NR-2 | **DNS server** | dnsmasq DNS disabled (`port=0`). |
| NR-3 | **Talos cluster lifecycle management** | No `talosctl`, no bootstrap, no upgrades, no node membership management. Boot/provisioning path only. Runtime config application via `talosctl apply-config` is the operator's responsibility. |
| NR-4 | **Secret generation** | System never generates secrets. Operator provides all certs/tokens via Vault. Supports existing clusters. |
| NR-5 | **Secure boot / TPM attestation** | Unsigned PXE boot. Future enhancement. |
| NR-6 | **Web UI / dashboard** | All config is file-based. Logs via container logging. |
| NR-7 | **Multi-VLAN / DHCP relay** | Single L2 broadcast domain. Network infra handles relay. |
| NR-8 | **Booting the Pi itself over PXE** | Pi is the server host, not a client. Pi boot is firmware config. |
| NR-9 | **Storage provisioning for Talos nodes** | Talos handles disk partitioning via machine config. |
| NR-10 | **High availability / multi-instance PXE** | Single instance per L2 segment. Running multiple PXE-in-a-Box instances simultaneously on the same L2 segment will cause proxy DHCP conflicts. Only one instance must be active at a time. Cold standby with shared config if needed. The system does not detect or prevent this — operator's responsibility. |
| NR-11 | **ARM64 PXE clients** | Container runs on ARM64, but serves x86_64 clients (BIOS and UEFI). ARM64 client support is future. |
| NR-12 | **OS disk installation for Ubuntu/Debian/Arch** | System delivers boot path only. OS installer/cloud-init handles disk install. |
| NR-13 | **Air-gapped asset mirroring** | Downloads from upstream. `cleanup: false` + manual pre-staging for air-gapped. |
| NR-14 | **Authentication on matchbox HTTP** | Matchbox has no auth. Security via network segmentation + bootstrap node pattern. |
| NR-15 | **Hot reload of configuration** | Ansible re-deploy is the reload mechanism. Predictable and auditable. |
| NR-16 | **Concurrent asset download coordination** | Single-instance deployment assumed. Shared volume race conditions are operator's responsibility. |
| NR-17 | **Zero-downtime config updates** | Container restart causes brief PXE interruption (seconds). Acceptable for homelab. |
