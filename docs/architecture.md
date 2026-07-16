# PXE-in-a-Box — Architecture & Design

> **Note:** This document describes the original architecture including the
> Ansible deployment design. The current implementation has replaced Ansible
> with in-container Go template rendering. See the README for the current
> deployment flow. The core PXE architecture (matchbox + dnsmasq + iPXE) is
> unchanged.

---

## 1. Architecture Overview

### 1.1 Design Constraint Summary

Based on matchbox and dnsmasq documentation research, the following constraints shape the architecture:

| Constraint | Source | Impact |
|-----------|--------|--------|
| `/boot.ipxe` is a hardcoded constant in matchbox | matchbox source (`ipxeBootstrap` Go const) | Cannot customize the entry point script |
| `/ipxe` renders a fixed Go template (kernel+initrd+boot) | matchbox source (`ipxeTemplate`) | Cannot serve interactive iPXE menus via profiles |
| No templating in kernel args | matchbox source (template only interpolates `{{.Kernel}}`, `{{.Args}}`, `{{.Initrd}}`) | `talos.config=` URL must be hardcoded per-machine in the profile |
| If no group matches, `/ipxe` returns HTTP 404 | matchbox source (`SelectGroup` returns error) | iPXE `chain` command fails, enabling `||` fallback |
| Groups read from disk on each request | matchbox source (`FileStore` has no cache) | Regenerating group JSON files takes effect without restart (but we restart anyway for consistency) |
| Assets served via Go FileServer at `/assets/` | matchbox source (`http.FileServer`) | Can serve custom static files (including iPXE scripts) alongside kernel/initrd |
| dnsmasq `pxe-service` can point to HTTP URLs | dnsmasq man page | Can redirect iPXE clients to any HTTP URL, not just matchbox's `/boot.ipxe` |
| iPXE `chain` supports `\|\|` error handling | iPXE scripting docs | If `chain` fails (HTTP 404), iPXE falls through to next command |

### 1.2 The Key Design Decision: Custom boot.ipxe with 404 Fallback

Matchbox cannot serve an iPXE menu. But iPXE's `chain` command supports `||` error handling. The architecture exploits this:

1. A **custom `boot.ipxe`** script is generated at deploy time and served as a static file by matchbox's `/assets/` path
2. This script attempts to `chain` to matchbox's `/ipxe?mac=...` endpoint
3. For **known MACs**: matchbox matches a group, returns 200 with kernel+initrd+args → iPXE boots directly
4. For **unknown MACs**: matchbox returns 404 (no matching group) → `chain` fails → iPXE falls through to the menu section

The generated `boot.ipxe`:
```ipxe
#!ipxe
# Attempt to match against known machines via matchbox
chain http://MATCHBOX_ADDR:MATCHBOX_PORT/ipxe?mac=${mac:hexhyp}&uuid=${uuid}&hostname=${hostname}&serial=${serial} || goto menu

:menu
menu --timeout ${timeout} --default ${default} PXE Boot Menu
item talos Talos Linux (Maintenance Mode)
item ubuntu Ubuntu Server
choose --default ${default} --timeout ${timeout} target && goto ${target}

:talos
kernel /assets/talos-MAINTENANCE_ID/amd64/vmlinuz initrd=initramfs.xz talos.platform=metal talos.config=null console=tty0 printk.devkmsg=on
initrd /assets/talos-MAINTENANCE_ID/amd64/initramfs.xz
boot

:ubuntu
kernel /assets/ubuntu-ID/amd64/linux initrd=initrd root=/dev/ram0 ramdisk_size=1500000 ip=dhcp url=https://releases.ubuntu.com/...
initrd /assets/ubuntu-ID/amd64/initrd
boot
```

Values like `${timeout}`, `${default}`, asset IDs, and the matchbox address are resolved at generation time (by Ansible), not at iPXE runtime. The resulting script is static.

### 1.3 No Catch-All Group

Unlike the original requirements, there is **no catch-all group** in matchbox. If a catch-all group existed, matchbox would always return 200 (matching the catch-all), and the `chain` command would never fail. By omitting the catch-all, unknown MACs get 404, triggering the menu fallback.

### 1.4 Per-Machine Profiles

Matchbox does not support templating in kernel args. The `talos.config=http://<matchbox>/assets/rendered/<hostname>.yaml` URL must be hardcoded in each profile. Therefore, each machine gets its own profile JSON file, generated at deploy time.

### 1.5 Network Topology

```
                        ┌──────────────────────────────────────┐
                        │         UniFi UDM Pro                 │
                        │         (Main DHCP Server)            │
                        │                                      │
                        │  - Assigns IP addresses to clients   │
                        │  - DHCP option: next-server / TFTP   │
                        │    server = 192.168.2.103             │
                        │  - Default gateway, DNS for network   │
                        └──────────────┬───────────────────────┘
                                       │ DHCP offer with PXE info
                                       │ points client to 192.168.2.103
                                       ▼
    ┌──────────────────────────────────────────────────────────────────┐
    │                  192.168.2.103 (PXE Host)                        │
    │                                                                  │
    │            ┌─────────────────────────────────────────────┐       │
    │            │          Docker Container                   │       │
    │            │          (--net host)                       │       │
    │            │                                             │       │
    │            │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │       │
    │            │  │ dnsmasq  │ │ matchbox │ │ downloader  │ │       │
    │            │  │          │ │          │ │ (entrypoint)│ │       │
    │            │  │ proxyDHCP│ │ HTTP:8081│ │             │ │       │
    │            │  │ TFTP:69  │ │          │ │ validate    │ │       │
    │            │  └────┬─────┘ └────┬─────┘ │ download    │ │       │
    │            │       │            │        │ cleanup     │ │       │
    │            │       │   ┌────────┘        └─────────────┘ │       │
    │            │       │   │                                 │       │
    │            │  ┌────▼───▼───────────────────────────────┐ │       │
    │            │  │          Shared Volumes                 │ │       │
    │            │  │                                        │ │       │
    │            │  │  /config/            /assets/           │ │       │
    │            │  │  ├ dnsmasq.conf      ├ boot.ipxe        │ │       │
    │            │  │  ├ machines.yaml     ├ rendered/        │ │       │
    │            │  │  ├ assets.yaml       ├ talos-v*/amd64/  │ │       │
    │            │  │  ├ menu.yaml         └ ubuntu-*/amd64/  │ │       │
    │            │  │  ├ templates/                          │ │       │
    │            │  │  ├ groups/ (gen)      /tftpboot/ (img)  │ │       │
    │            │  │  ├ profiles/ (gen)    ├ undionly.kpxe   │ │       │
    │            │  │  └ static/            └ ipxe.efi        │ │       │
    │            │  └────────────────────────────────────────┘ │       │
    │            └─────────────────────────────────────────────┘       │
    └──────────────────────────────────────────────────────────────────┘
```

**Network responsibilities:**

| Component | Role | How it's involved |
|-----------|------|-------------------|
| **UDM Pro** | Main DHCP server | Assigns IPs, tells PXE clients "TFTP server = 192.168.2.103" via DHCP options. No changes needed to UDM Pro config. |
| **192.168.2.103** | PXE host | Static IP (DHCP reservation on UDM Pro). Runs the PXE-in-a-Box container. |
| **dnsmasq (in container)** | Proxy DHCP + TFTP | Proxy DHCP supplements UDM Pro's DHCP with PXE boot info. TFTP serves iPXE binaries. Uses `--net host` for L2 broadcast access. |
| **matchbox (in container)** | HTTP server | Serves boot scripts, kernels, configs on port 8081. With `--net host`, no port mapping needed. |

**Why two DHCP servers don't conflict:** The UDM Pro is the primary DHCP server (assigns IPs). dnsmasq runs in **proxy DHCP mode** — it doesn't assign IPs, it only adds PXE boot information to DHCP responses. Both can coexist on the same network without conflict.

### 1.6 Request Flow

#### Known Machine (zero-touch boot)
```
Machine PXE boot
    │
    ▼
UDM Pro DHCP ──► assigns IP, points to TFTP server (192.168.2.103)
    │
    ▼
dnsmasq proxy DHCP ──► TFTP: undionly.kpxe
    │
    ▼
iPXE loads, fetches http://192.168.2.103:8081/assets/boot.ipxe
    │
    ▼
boot.ipxe: chain http://192.168.2.103:8081/ipxe?mac=00:e0:4c:68:00:8e&...
    │
    ▼
matchbox: group "worker01" matches MAC → profile "talos-v1.10.6-worker01"
    │
    ▼
matchbox returns 200:
    #!ipxe
    kernel /assets/talos-v1.10.6/amd64/vmlinuz ... talos.config=http://192.168.2.103:8081/assets/rendered/worker01.yaml
    initrd /assets/talos-v1.10.6/amd64/initramfs.xz
    boot
    │
    ▼
iPXE loads kernel + initrd, boots Talos
    │
    ▼
Talos fetches http://192.168.2.103:8081/assets/rendered/worker01.yaml
    │
    ▼
Talos reads machine config, installs to disk, joins cluster
```

#### Unknown Machine (menu fallback)
```
Machine PXE boot
    │
    ▼
UDM Pro DHCP ──► assigns IP, points to TFTP server (192.168.2.103)
    │
    ▼
dnsmasq proxy DHCP ──► TFTP: undionly.kpxe
    │
    ▼
iPXE loads, fetches http://MATCHBOX:PORT/assets/boot.ipxe
    │
    ▼
boot.ipxe: chain http://MATCHBOX:PORT/ipxe?mac=aa:bb:cc:dd:ee:ff&...
    │
    ▼
matchbox: no group matches → returns HTTP 404
    │
    ▼
chain fails, iPXE falls through via || goto menu
    │
    ▼
iPXE displays menu:
    1. Talos Linux (Maintenance Mode)  [default, 10s timeout]
    2. Ubuntu Server
    │
    ▼
Timeout or selection → boots selected OS
    (Talos: talos.config=null → maintenance mode, no cluster join)
```

---

## 2. Component Design

### 2.1 dnsmasq

#### Configuration

The `dnsmasq.conf` is mostly unchanged from the existing setup. The key change: the iPXE HTTP redirect points to `/assets/boot.ipxe` (our custom script) instead of matchbox's hardcoded `/boot.ipxe`.

```conf
# Proxy DHCP — no IP leases. UDM Pro handles IP assignment.
# dnsmasq only supplements with PXE boot information.
dhcp-range=192.168.0.1,proxy,255.255.0.0

# Disable DNS (UDM Pro handles DNS for the network)
port=0

# TFTP
enable-tftp
tftp-root=/tftpboot

# Detect iPXE user-class (sets "ipxe" tag on iPXE DHCP requests)
dhcp-userclass=set:ipxe,iPXE

# ── Stage 1: Raw PXE clients get iPXE binary via TFTP ──
# tag:#ipxe means "NOT tagged as ipxe" (raw PXE client, first boot)
# x86PC = BIOS clients, x86-64_EFI = UEFI clients

pxe-service=tag:#ipxe,x86PC,"PXE chainload to iPXE",undionly.kpxe
pxe-service=tag:#ipxe,x86-64_EFI,"PXE chainload to iPXE",ipxe.efi

# ── Stage 2: iPXE clients get HTTP boot script ──
# Once iPXE is loaded, it re-DHCPs with user-class "iPXE".
# dnsmasq matches the ipxe tag and serves the HTTP URL instead of TFTP.
# CHANGED: /boot.ipxe → /assets/boot.ipxe (custom script with menu fallback)

pxe-service=tag:ipxe,x86PC,"iPXE",http://192.168.2.103:8081/assets/boot.ipxe
pxe-service=tag:ipxe,x86-64_EFI,"iPXE",http://192.168.2.103:8081/assets/boot.ipxe

# Logging
log-queries
log-dhcp
```

**Design notes:**
- `192.168.2.103` and port `8081` are resolved at deploy time by Ansible (substituted into the config from Ansible vars). They are not runtime variables.
- The only change from the current working `dnsmasq.conf` is the HTTP URL: `/boot.ipxe` → `/assets/boot.ipxe`. Everything else stays the same.
- The UDM Pro is the primary DHCP server — it assigns IPs and tells clients where the TFTP server is. dnsmasq proxy DHCP only adds PXE boot info. The two do not conflict.
- The `tag:#ipxe` syntax means "not tagged as ipxe" — so raw PXE clients get the TFTP binary, and iPXE clients get the HTTP URL. This is the two-stage chainload.

#### UDM Pro Configuration

**No changes needed to the UDM Pro.** It already points TFTP to 192.168.2.103. The required UDM Pro settings (already in place):

| Setting | Value | Where |
|---------|-------|-------|
| DHCP server | Active (UDM Pro manages IP assignment) | Network settings |
| TFTP server / next-server | `192.168.2.103` | DHCP options / network config |
| DHCP reservation for PXE host | `192.168.2.103` (static lease for PXE host MAC) | DHCP reservations |

#### BIOS and UEFI Support

The system handles both BIOS and UEFI PXE clients simultaneously. dnsmasq's
`pxe-service` directives use CSA (Client System Architecture) types to serve
the correct iPXE binary per architecture.

| Client State | CSA Type | What dnsmasq serves |
|---|---|---|
| Raw PXE, BIOS | `x86PC` | `undionly.kpxe` via TFTP |
| Raw PXE, UEFI | `x86-64_EFI` | `ipxe.efi` via TFTP |
| iPXE, BIOS | `x86PC` | HTTP boot script URL |
| iPXE, UEFI | `x86-64_EFI` | HTTP boot script URL |

After iPXE loads, both BIOS and UEFI clients fetch the same HTTP boot script
and follow the same path. The architecture divergence ends at stage 1.

### 2.2 matchbox

#### Startup Configuration

```
matchbox \
  -address 0.0.0.0:8081 \
  -data-path /config \
  -assets-path /assets \
  -log-level debug
```

**Key changes from existing setup:**
- `-data-path /config` — groups, profiles read from `/config/groups/`, `/config/profiles/`
- `-assets-path /assets` — static assets (kernels, initramfs, rendered configs, boot.ipxe) served from `/assets/`
- No gRPC endpoint (not needed for file-based config)
- No TLS by default (optional via config)

#### What matchbox serves

| URL | Source | Purpose |
|-----|--------|---------|
| `/boot.ipxe` | Hardcoded constant | **Not used** — dnsmasq points to `/assets/boot.ipxe` instead |
| `/ipxe?mac=...` | Generated from profile | Returns iPXE kernel+initrd+boot script for matched MACs. Returns 404 for unknown MACs. |
| `/assets/boot.ipxe` | Static file (generated) | Custom boot script with 404 fallback to menu |
| `/assets/rendered/*.yaml` | Static file (generated) | Rendered Talos machine configs |
| `/assets/talos-*/amd64/*` | Static file (downloaded) | Kernel and initramfs binaries |
| `/assets/ubuntu-*/amd64/*` | Static file (downloaded) | Ubuntu netboot artifacts |

#### Group files (generated)

Each machine in `machines.yaml` generates one group JSON file. **No catch-all group is generated.**

Example `/config/groups/worker01.json`:
```json
{
  "id": "worker01",
  "name": "worker01",
  "profile": "talos-v1.10.6-worker01",
  "selector": {
    "mac": "00:e0:4c:68:00:a1"
  }
}
```

Matchbox's `SelectGroup` sorts groups by number of selectors (descending). All our groups have exactly one selector (`mac`), so they're tried in alphabetical order. Since each MAC is unique, exactly one group matches per known machine. Unknown MACs match nothing → 404.

#### Profile files (generated)

Each machine generates one profile JSON file. The profile contains the per-machine `talos.config` URL hardcoded in the kernel args.

Example `/config/profiles/talos-v1.10.6-worker01.json`:
```json
{
  "id": "talos-v1.10.6-worker01",
  "name": "Talos v1.10.6 - worker01",
  "boot": {
    "kernel": "/assets/talos-v1.10.6/amd64/vmlinuz",
    "initrd": ["/assets/talos-v1.10.6/amd64/initramfs.xz"],
    "args": [
      "initrd=initramfs.xz",
      "init_on_alloc=1",
      "slab_nomerge",
      "pti=on",
      "console=tty0",
      "printk.devkmsg=on",
      "talos.platform=metal",
      "talos.config=http://192.168.2.103:8081/assets/rendered/worker01.yaml"
    ]
  }
}
```

For singleton machines with a static config file, the `talos.config` URL points to the static file instead:
```json
{
  "id": "talos-v1.9.1-melfina",
  "name": "Talos v1.9.1 - melfina",
  "boot": {
    "kernel": "/assets/talos-v1.9.1/amd64/vmlinuz",
    "initrd": ["/assets/talos-v1.9.1/amd64/initramfs.xz"],
    "args": [
      "initrd=initramfs.xz",
      "init_on_alloc=1",
      "slab_nomerge",
      "pti=on",
      "console=tty0",
      "printk.devkmsg=on",
      "talos.platform=metal",
      "talos.config=http://192.168.2.103:8081/assets/static/melfina.yaml"
    ]
  }
}
```

### 2.3 Custom boot.ipxe Generator

The `boot.ipxe` file is generated at deploy time by Ansible from `menu.yaml` + the matchbox endpoint address. It is placed in the assets directory at `/assets/boot.ipxe`.

**Generation logic:**
1. Read `menu.yaml` for timeout, default entry, and menu items
2. Read matchbox endpoint address from Ansible vars
3. Read the maintenance Talos asset ID (for the Talos menu entry)
4. Read the Ubuntu asset ID (for the Ubuntu menu entry)
5. Render the iPXE script using a Jinja2 template

**Template (`boot.ipxe.j2`):**
```ipxe
#!ipxe
# Generated by PXE-in-a-Box. Do not edit manually.
# Attempts to match against known machines via matchbox.
# If matchbox returns 404 (unknown MAC), falls through to boot menu.
# Note: if a known machine's kernel boot fails, chain also falls through to menu.

chain http://{{ matchbox_addr }}:{{ matchbox_port }}/ipxe?mac=${mac:hexhyp}&uuid=${uuid}&hostname=${hostname}&serial=${serial} || goto menu

:menu
menu PXE Boot Menu
{% for entry in menu_entries %}
item {{ entry.id }} {{ entry.label }}
{% endfor %}
choose --default {{ menu_default }} --timeout {{ menu_timeout * 1000 }} target && goto ${target}

{% for entry in menu_entries %}
:{{ entry.id }}
{% if entry.uki %}
kernel /assets/{{ entry.asset_id }}/amd64/{{ entry.uki_filename }} {{ entry.args | join(" ") }}
{% else %}
kernel /assets/{{ entry.asset_id }}/amd64/{{ entry.kernel_filename }} {{ entry.args | join(" ") }}
initrd /assets/{{ entry.asset_id }}/amd64/{{ entry.initrd_filename }}
{% endif %}
boot

{% endfor %}
```

**Rendered example:**
```ipxe
#!ipxe
# Generated by PXE-in-a-Box. Do not edit manually.
# Attempts to match against known machines via matchbox.
# If matchbox returns 404 (unknown MAC), falls through to boot menu.
# Note: if a known machine's kernel boot fails, chain also falls through to menu.

chain http://192.168.2.103:8081/ipxe?mac=${mac:hexhyp}&uuid=${uuid}&hostname=${hostname}&serial=${serial} || goto menu

:menu
menu PXE Boot Menu
item talos Talos Linux (Maintenance Mode)
item ubuntu Ubuntu Server 24.04
choose --default talos --timeout 10000 target && goto ${target}

:talos
kernel /assets/talos-v1.10.6/amd64/vmlinuz initrd=initramfs.xz talos.platform=metal talos.config=null console=tty0 printk.devkmsg=on
initrd /assets/talos-v1.10.6/amd64/initramfs.xz
boot

:ubuntu
kernel /assets/ubuntu-noble/amd64/linux initrd=initrd root=/dev/ram0 ramdisk_size=1500000 ip=dhcp url=https://releases.ubuntu.com/24.04.1/ubuntu-24.04.1-live-server-amd64.iso
initrd /assets/ubuntu-noble/amd64/initrd
boot
```

### 2.4 Asset Downloader

Runs as the first phase of the container entrypoint, before dnsmasq and matchbox start.

**Flow:**
```
Read assets.yaml
    │
    ▼
For each OS type (talos, ubuntu, debian, arch):
    For each entry:
        If asset directory exists and files present:
            Verify size (skip if OK)
        Else:
            Download from upstream
            Verify checksum (if available)
            Retry up to 3 times on failure
    │
    ▼
If cleanup enabled:
    Scan /assets/ for directories not in manifest
    Skip protected paths (rendered/, boot.ipxe)
    Delete and log
    │
    ▼
Log startup summary:
    - Available assets (id, path, size)
    - Failed downloads (id, error)
    - Affected machines (which profiles reference missing assets)
```

**Download sources:**

| OS | Source URL | Files |
|----|-----------|-------|
| Talos (version) | `https://github.com/siderolabs/talos/releases/download/<version>/` | `vmlinuz-amd64`, `initramfs-amd64.xz` |
| Talos (image factory) | `https://factory.talos.dev/image/<hash>/<version>/<arch>` | `vmlinuz`, `initramfs.xz`, optionally `metal-amd64-uki.efi` |
| Ubuntu | `http://archive.ubuntu.com/ubuntu/dists/<release>/main/installer-amd64/current/legacy-images/netboot/ubuntu-installer/amd64/` | `linux`, `initrd`, `pxelinux.0`, `ldlinux.c32`, `bootx64.efi`, `grubx64.efi` |
| Debian | `https://deb.debian.org/debian/dists/<release>/main/installer-amd64/current/images/netboot/debian-installer/amd64/` | `linux`, `initrd.gz` |
| Arch | `https://mirror.example.com/archlinux/iso/latest/` or `archboot` | `vmlinuz-linux`, `initramfs-linux.img` |

**File naming convention:** Downloaded files are renamed to match the path convention:
- Talos: `vmlinuz-amd64` → `vmlinuz`, `initramfs-amd64.xz` → `initramfs.xz`
- Ubuntu: files keep upstream names
- Debian: `initrd.gz` stays as `initrd.gz`
- Arch: files keep upstream names

**Protected paths during cleanup:**
- `/assets/rendered/` — generated machine configs
- `/assets/boot.ipxe` — generated boot script
- `/assets/static/` — singleton config files (if placed here)

These are not in the asset manifest but must not be deleted.

### 2.5 Process Supervisor

Uses **s6-overlay** (Alpine-compatible, lightweight, proven in Docker containers).

**Services:**

| Service | Command | Restart |
|---------|---------|---------|
| `dnsmasq` | `dnsmasq --no-daemon --conf-file=/config/dnsmasq.conf` | yes |
| `matchbox` | `/matchbox -address 0.0.0.0:8081 -data-path /config -assets-path /assets -log-level $LOG_LEVEL` | yes |

**Entrypoint sequence:**
1. Run config validation script
2. Run asset downloader script
3. Run optional cleanup
4. Log startup summary
5. Start s6-overlay (manages dnsmasq + matchbox)
6. s6-overlay handles signals, restarts, and clean shutdown

### 2.6 Config Validator

A script (Python or shell) that runs before any services start. Validates:

1. **`machines.yaml` schema:**
   - Required fields: `mac`, `hostname`, `profile` (or `template` for groups)
   - Valid MAC address format
   - No duplicate MACs across all groups and singletons
   - Referenced templates exist in `/config/templates/`
   - Referenced static configs exist in `/config/static/`

2. **`assets.yaml` schema:**
   - Valid OS types (talos, ubuntu, debian, arch)
   - Required fields per OS type (version or image_factory_hash for talos, release for ubuntu/debian)
   - No duplicate IDs
   - At least one Talos asset for maintenance mode

3. **`menu.yaml` schema:**
   - Timeout is a positive integer
   - Default entry exists in entries list
   - Each entry references a valid profile/asset

4. **Cross-references:**
   - Every `profile` referenced in `machines.yaml` has a corresponding asset in `assets.yaml`
   - The maintenance profile referenced in `menu.yaml` has a corresponding Talos asset

On validation failure: print clear error message, exit non-zero. Container fails to start.

---

## 3. Ansible Playbook Design

### 3.1 Directory Structure

```
pxe-in-a-box/
├── ansible/
│   ├── site.yml                    # Main playbook
│   ├── inventory.ini               # Target hosts
│   ├── group_vars/
│   │   └── all.yml                 # Shared variables
│   ├── group_vars/
│   │   └── vault.yml               # Encrypted secrets (ansible-vault)
│   ├── roles/
│   │   ├── pxe-validate/           # Pre-validation role
│   │   ├── pxe-render/             # Template rendering + group/profile generation
│   │   ├── pxe-deploy/             # Docker image pull + container management
│   │   └── pxe-assets/             # Asset manifest management
│   └── files/
│       └── config/
│           ├── dnsmasq.conf.j2     # dnsmasq config template
│           ├── machines.yaml       # Machine definitions (operator-edited)
│           ├── assets.yaml         # Asset manifest (operator-edited)
│           ├── menu.yaml           # Boot menu (operator-edited)
│           ├── templates/          # Jinja2 Talos config templates
│           │   ├── controlplane.yaml.j2
│           │   └── worker.yaml.j2
│           └── static/             # One-off configs for singletons
```

### 3.2 Playbook Flow

```
site.yml
    │
    ▼
1. pxe-validate (local, on Ansible controller)
   - Validate machines.yaml schema
   - Validate assets.yaml schema
   - Validate menu.yaml schema
   - Check for duplicate MACs
   - If FAIL → abort playbook, don't touch target
    │
    ▼
2. pxe-render (local, on Ansible controller)
   - Clear output directories (profiles/, groups/, rendered/)
   - For each group in machines.yaml:
     - For each machine in group:
       - Merge group vars + machine vars
       - Render template with Vault secrets → /rendered/<hostname>.yaml
       - Generate profile JSON → /profiles/<asset-id>-<hostname>.json
       - Generate group JSON → /groups/<hostname>.json
   - For each singleton in machines.yaml:
     - Generate profile JSON → /profiles/<asset-id>-<hostname>.json
     - Generate group JSON → /groups/<hostname>.json
   - Generate boot.ipxe from menu.yaml + matchbox endpoint
    │
    ▼
3. pxe-deploy (on target host)
   - Install Docker if needed
   - Create config directory structure
   - Copy rendered config files to target
   - Pull Docker image
   - Stop existing container if running
   - Start new container with mounted volumes
    │
    ▼
4. pxe-assets (on target host, via container)
   - Container entrypoint handles asset download/cleanup
   - (This is the container's job, not Ansible's)
```

### 3.3 Key Ansible Variables

```yaml
# group_vars/all.yml
pxe_image: ghcr.io/<user>/pxe-in-a-box:v1.0.0
pxe_container_name: pxe-in-a-box
pxe_config_dir: /opt/pxe/config
pxe_assets_dir: /opt/pxe/assets

# PXE host address (static IP assigned by UDM Pro DHCP reservation)
matchbox_addr: 192.168.2.103
matchbox_port: 8081

# dnsmasq proxy DHCP range (must cover the network where PXE clients live)
dnsmasq_proxy_range: 192.168.0.1
dnsmasq_proxy_netmask: 255.255.0.0

menu_timeout: 10
menu_default: talos

# group_vars/vault.yml (encrypted)
talos_ca_cert: "LS0tLS1CRUdJTi..."
talos_ca_key: "LS0tLS1CRUdJTi..."
talos_join_token: "REDACTED_JOIN_TOKEN"
talos_etcd_ca_cert: "LS0tLS1CRUdJTi..."
talos_etcd_ca_key: "LS0tLS1CRUdJTi..."
```

---

## 4. Docker Image Design

### 4.1 Base Image

**Alpine Linux** (multi-arch: amd64, arm64). Lightweight (~5MB base), good s6-overlay support, dnsmasq available in repos.

### 4.2 Image Contents

```
# Baked into image (stable, rarely changes):
/tftpboot/undionly.kpxe          # iPXE BIOS chainload
/tftpboot/ipxe.efi               # iPXE UEFI chainload
/matchbox                         # matchbox binary (multi-arch)
/usr/sbin/dnsmasq                # dnsmasq binary
/config-validator                # Validation script
/asset-downloader                # Download script
/s6-overlay/                     # Process supervisor

# NOT in image (mounted volumes):
/config/                         # All operator config
/assets/                         # All downloaded/generated assets
```

### 4.3 iPXE Binaries

Downloaded at image build time from `http://boot.ipxe.org/` or built from source. Both are x86_64 binaries (the PXE clients are x86_64, even when the container runs on ARM64).

| File | Architecture | Purpose | Status |
|------|-------------|---------|--------|
| `undionly.kpxe` | x86_64 | BIOS PXE chainload to iPXE | **Active** — used by dnsmasq config |
| `ipxe.efi` | x86_64 | UEFI PXE chainload to iPXE | **Active** — used by UEFI clients |

### 4.4 Multi-Arch Build

Built with `docker buildx`:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag ghcr.io/<user>/pxe-in-a-box:v1.0.0 \
  --push .
```

**Per-architecture differences:**
- `matchbox` binary: Download pre-built for each arch (if available) or build from source
- `dnsmasq`: Available in both Alpine amd64 and arm64 repos
- `s6-overlay`: Available for both arches
- iPXE binaries: Always x86_64 (clients are x86_64 regardless of host arch)

### 4.5 Health Check

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O- http://localhost:8081/ && \
      nc -z -u localhost 69 || exit 1
```

Checks:
- matchbox HTTP responds on port 8081
- TFTP port 69 is listening (UDP check via `nc`)

### 4.6 Dockerfile (Conceptual)

```dockerfile
FROM alpine:3.20

# Install dnsmasq and utilities
RUN apk add --no-cache dnsmasq wget curl python3 py3-yaml

# Install s6-overlay
COPY --from=s6-overlay / /

# Install matchbox binary (per-arch, downloaded at build time)
ARG TARGETARCH
COPY matchbox-${TARGETARCH} /matchbox
RUN chmod +x /matchbox

# Copy iPXE binaries (always x86_64)
COPY tftpboot/ /tftpboot/

# Copy scripts
COPY scripts/ /scripts/
RUN chmod +x /scripts/*.py

# Volumes
VOLUME ["/config", "/assets"]

# Entrypoint
ENTRYPOINT ["/scripts/entrypoint.py"]
```

---

## 5. Data Flow Diagram

### 5.1 Deploy Flow (Ansible)

```
Operator edits Git
    │
    ├── machines.yaml
    ├── assets.yaml
    ├── menu.yaml
    ├── templates/*.j2
    └── group_vars/vault.yml (encrypted)
    │
    ▼
ansible-playbook site.yml --ask-vault-pass
    │
    ├─► [Controller] Validate config locally
    │     - Schema check
    │     - Duplicate MAC check
    │     - Cross-reference check
    │     FAIL → abort, target untouched
    │
    ├─► [Controller] Render templates
    │     - Decrypt Vault secrets
    │     - For each machine: merge vars → render Jinja2 → write rendered/<hostname>.yaml
    │     - For each machine: generate profile JSON (with hardcoded talos.config URL)
    │     - For each machine: generate group JSON (with MAC selector)
    │     - Generate boot.ipxe from menu.yaml
    │     - Clear stale files in profiles/, groups/, rendered/
    │
    ├─► [Target] Copy config to target host
    │     - /config/dnsmasq.conf (rendered from template)
    │     - /config/machines.yaml
    │     - /config/assets.yaml
    │     - /config/menu.yaml
    │     - /config/templates/ (copied for reference)
    │     - /config/groups/ (generated)
    │     - /config/profiles/ (generated)
    │     - /assets/rendered/ (generated configs)
    │     - /assets/boot.ipxe (generated)
    │     - /assets/static/ (singleton configs)
    │
    └─► [Target] Pull image, restart container
          - docker pull <image>:<version>
          - docker stop pxe-in-a-box (if running)
          - docker run -d \
              --name pxe-in-a-box \
              --net host \
              -v /opt/pxe/config:/config \
              -v /opt/pxe/assets:/assets \
              -e LOG_LEVEL=debug \
              <image>:<version>
                │
                ▼
          Container entrypoint:
            1. Validate config (second line of defense)
            2. Download missing assets
            3. Optional cleanup
            4. Log startup summary
            5. Start dnsmasq + matchbox via s6
```

### 5.2 Runtime Data Flow

```
                    ┌──────────────────┐
                    │  UDM Pro         │
                    │  (DHCP Server)   │
                    │  assigns IP +    │
                    │  TFTP=192.168.   │
                    │  2.103           │
                    └────────┬─────────┘
                             │ DHCP offer with PXE info
                             ▼
 ┌────────────┐    PXE     ┌──────────────┐    TFTP     ┌──────────────┐
 │  Machine   │───────────►│   dnsmasq    │────────────►│ /tftpboot    │
 │ (PXE client)│           │  (proxy DHCP)│             │ undionly.kpxe│
 └────────────┘            └──────┬───────┘             └──────────────┘
      │                           │
      │ ◄── TFTP: iPXE binary ───┘
      │
      │ HTTP: GET /assets/boot.ipxe
      ▼
┌──────────────┐
│   matchbox   │
│  (HTTP:8081) │
└──────┬───────┘
       │
       ├── boot.ipxe served (static file)
       │
       │ iPXE executes: chain /ipxe?mac=...
       │
       ├── /ipxe?mac=<known>
       │   ├── Group matches → Profile found
       │   ├── Return 200: kernel + initrd + args (with talos.config=...)
       │   └── iPXE boots kernel
       │        ├── HTTP: GET /assets/talos-*/vmlinuz
       │        ├── HTTP: GET /assets/talos-*/initramfs.xz
       │        └── Talos boots, fetches /assets/rendered/<hostname>.yaml
       │             └── Talos installs, joins cluster
       │
       └── /ipxe?mac=<unknown>
           ├── No group matches → Return 404
           └── iPXE falls through to menu
                ├── Display menu (10s timeout)
                ├── Default: Talos maintenance
                │    ├── HTTP: GET /assets/talos-*/vmlinuz
                │    ├── HTTP: GET /assets/talos-*/initramfs.xz
                │    └── Talos boots (talos.config=null, maintenance mode)
                └── Optional: Ubuntu
                     ├── HTTP: GET /assets/ubuntu-*/linux
                     └── HTTP: GET /assets/ubuntu-*/initrd
```

---

## 6. Volume Layout (Final)

### 6.1 /config/ (mounted, read-only at runtime)

```
/config/
├── dnsmasq.conf                  # Rendered from dnsmasq.conf.j2 by Ansible
├── assets.yaml                   # Operator-edited asset manifest
├── machines.yaml                 # Operator-edited machine definitions
├── menu.yaml                     # Operator-edited boot menu config
├── templates/                    # Jinja2 templates (for reference/documentation)
│   ├── controlplane.yaml.j2
│   └── worker.yaml.j2
├── groups/                       # Generated by Ansible (cleared each deploy)
│   ├── cp00.json
│   ├── cp01.json
│   ├── worker01.json
│   └── worker02.json
├── profiles/                     # Generated by Ansible (cleared each deploy)
│   ├── talos-v1.10.6-cp00.json
│   ├── talos-v1.10.6-cp01.json
│   ├── talos-v1.10.6-worker01.json
│   └── talos-v1.10.6-worker02.json
└── static/                       # One-off configs for singletons
    └── melfina.yaml
```

### 6.2 /assets/ (mounted, read-write for downloads)

```
/assets/
├── boot.ipxe                     # Generated by Ansible (from menu.yaml)
├── rendered/                     # Generated by Ansible (from templates + Vault)
│   ├── cp00.yaml
│   ├── cp01.yaml
│   ├── worker01.yaml
│   └── worker02.yaml
├── talos-v1.10.6/                # Downloaded by asset-downloader
│   └── amd64/
│       ├── vmlinuz
│       └── initramfs.xz
├── talos-v1.9.1/                 # Downloaded by asset-downloader
│   └── amd64/
│       ├── vmlinuz
│       └── initramfs.xz
├── talos-nvidia/                 # Downloaded from Image Factory
│   └── amd64/
│       ├── vmlinuz
│       ├── initramfs.xz
│       └── metal-amd64-uki.efi  # if download_uki: true
└── ubuntu-noble/                 # Downloaded by asset-downloader
    └── amd64/
        ├── linux
        ├── initrd
        ├── pxelinux.0
        ├── ldlinux.c32
        ├── bootx64.efi
        └── grubx64.efi
```

### 6.3 /tftpboot/ (baked into image, read-only)

```
/tftpboot/
├── undionly.kpxe                 # BIOS iPXE chainload (x86_64) — ACTIVE
└── ipxe.efi                      # UEFI iPXE chainload (x86_64) — ACTIVE
```

---

## 7. Security Architecture

### 7.1 Layered Defense

```
Layer 1: Network Segmentation (Primary)
    │ PXE VLAN isolated from main network
    │ Only PXE server + booting machines on VLAN
    │ Router/firewall prevents access from other VLANs
    │
Layer 2: Secret Lifecycle (Ansible Vault)
    │ Secrets encrypted in Git
    │ Decrypted only at deploy time on Ansible controller
    │ Rendered configs (plaintext) exist only on PXE host's /assets/ volume
    │ Not in Git, not in Docker image
    │
Layer 3: Bootstrap Node Pattern
    │ First control plane config (with CA key) applied out-of-band
    │ Subsequent configs contain only CA cert (public) + join token
    │ Limits CA key exposure to one manual step
    │
Layer 4: Matchbox TLS (Optional)
    │ HTTPS for talos.config URL
    │ Custom iPXE build with CA cert embedded (or publicly trusted cert)
    │ Prevents passive sniffing
```

### 7.2 What's on Disk vs. in Git

| Data | Git | Ansible Controller | PXE Host Disk | Docker Image |
|------|-----|-------------------|---------------|-------------|
| `machines.yaml` | ✅ plaintext | ✅ plaintext | ✅ plaintext | ❌ |
| `assets.yaml` | ✅ plaintext | ✅ plaintext | ✅ plaintext | ❌ |
| `menu.yaml` | ✅ plaintext | ✅ plaintext | ✅ plaintext | ❌ |
| Templates (`*.j2`) | ✅ plaintext | ✅ plaintext | ✅ plaintext (reference) | ❌ |
| Vault secrets | ✅ encrypted | ✅ decrypted (transient) | ❌ | ❌ |
| Rendered configs | ❌ | ✅ transient | ✅ plaintext (`/assets/rendered/`) | ❌ |
| Group/profile JSON | ❌ | ✅ transient | ✅ plaintext (`/config/groups/`, `/config/profiles/`) | ❌ |
| `boot.ipxe` | ❌ | ✅ transient | ✅ plaintext (`/assets/boot.ipxe`) | ❌ |
| Kernel/initramfs | ❌ | ❌ | ✅ downloaded (`/assets/`) | ❌ |
| iPXE binaries | ❌ | ❌ | ❌ | ✅ baked in (`/tftpboot/`) |
| matchbox/dnsmasq | ❌ | ❌ | ❌ | ✅ baked in |

---

## 8. Design Decisions & Rationale

### 8.1 Why Custom boot.ipxe Instead of matchbox's /boot.ipxe?

Matchbox's `/boot.ipxe` is a hardcoded constant that chains to `/ipxe?...`. It cannot be customized. By serving our own `boot.ipxe` as a static file at `/assets/boot.ipxe`, we:
- Add the `|| goto menu` fallback for unknown machines
- Keep matchbox's group/profile matching for known machines (via `chain /ipxe?...`)
- Don't modify matchbox's source code

### 8.2 Why No Catch-All Group?

A catch-all group (empty selector) would match all unknown MACs and return 200 with a profile. This would prevent the `chain` command from failing, so the menu would never appear. By omitting the catch-all, unknown MACs get 404, triggering the `|| goto menu` fallback.

### 8.3 Why Per-Machine Profiles?

Matchbox's `/ipxe` template only interpolates `{{.Kernel}}`, `{{.Args}}`, and `{{.Initrd}}` from the profile. It does not support per-request templating in kernel args. Since each machine needs a unique `talos.config=http://.../rendered/<hostname>.yaml` URL, each machine must have its own profile with the URL hardcoded. Profiles are auto-generated, so this is invisible to the operator.

### 8.4 Why /assets/rendered/ Instead of matchbox's /generic/ or /ignition/?

Matchbox's `/generic/` and `/ignition/` endpoints go through group matching and Go template rendering. We want to serve pre-rendered static files (rendered by Ansible with Jinja2). The `/assets/` path is a simple file server with no processing. Putting rendered configs in `/assets/rendered/` is the simplest approach that avoids matchbox's template engine entirely.

### 8.5 Why Alpine Instead of Debian?

- Smaller base image (~5MB vs ~80MB)
- Native s6-overlay support
- dnsmasq available in repos
- matchbox is a single static binary (works on any Linux)
- Python available for scripts (via `apk add python3`)

### 8.6 Why host Network Mode?

dnsmasq needs to receive PXE/DHCP broadcast packets on the L2 network. Bridge networking doesn't pass broadcasts. `--net host` is the simplest way to enable this. With host networking, matchbox binds directly to `0.0.0.0:8081` — no port mapping needed. In Kubernetes, `hostNetwork: true` serves the same purpose.

This differs from the current setup which uses two containers: dnsmasq on `--net host` and matchbox on bridge with port mapping (8081→8080). PXE-in-a-Box merges both into one container with `--net host`, eliminating the bridge/publish complexity.

### 8.7 Why s6-overlay Instead of supervisord?

- Lighter weight (C, no Python runtime needed for the supervisor itself)
- Native Alpine support
- Simpler configuration
- Handles signal forwarding and clean shutdown
- Well-proven in Docker containers

---

## 9. File Generation Map

This table shows every generated file, its source, and its destination:

| File | Generated By | Source Inputs | Destination |
|------|-------------|---------------|-------------|
| `/config/dnsmasq.conf` | Ansible | `dnsmasq.conf.j2` + vars | Target host |
| `/config/groups/<hostname>.json` | Ansible | `machines.yaml` | Target host |
| `/config/profiles/<asset-id>-<hostname>.json` | Ansible | `machines.yaml` + `assets.yaml` + matchbox endpoint | Target host |
| `/assets/boot.ipxe` | Ansible | `menu.yaml` + matchbox endpoint + asset IDs | Target host |
| `/assets/rendered/<hostname>.yaml` | Ansible | `templates/*.j2` + `machines.yaml` vars + Vault secrets | Target host |
| `/assets/static/<config>.yaml` | Operator | Manual | Target host (copied by Ansible) |
| `/assets/<asset-id>/amd64/*` | Container entrypoint | `assets.yaml` + upstream sources | Target host (downloaded at container start) |

---

## 10. Open Questions & Risks

### 10.1 iPXE `chain || goto` Behavior with HTTP 404

**Status: TESTED AND VERIFIED via QEMU.**

Tested with QEMU's built-in iPXE (v1.21.1+). The `chain || goto menu` pattern works correctly:
- HTTP 404 from matchbox (unknown MAC) → `chain` fails → `|| goto menu` executes → menu appears
- HTTP 200 from matchbox (known MAC) → `chain` succeeds → matchbox's iPXE script executes → kernel boots
- If the chained script's `boot` fails (e.g., kernel not found) → `chain` also fails → falls through to menu

**Note:** The `menu` command does NOT accept `--timeout` or `--default` flags. These go on the `choose` command:
```
menu PXE Boot Menu
item talos Talos Linux
choose --default talos --timeout 5000 target && goto ${target}
```

### 10.2 Matchbox Binary on ARM64

**Risk:** Pre-built matchbox binaries may not be available for ARM64 (for Raspberry Pi deployment).

**Mitigation:** Check `quay.io/poseidon/matchbox` for ARM64 support. If not available, build matchbox from source in a multi-stage Docker build (Go cross-compiles to ARM64 easily).

**Status:** Needs verification. The existing Docker image (`quay.io/poseidon/matchbox:latest`) may or may not have ARM64 variants.

### 10.3 Ubuntu Netboot URL Stability

**Risk:** Ubuntu netboot URLs change between releases and Ubuntu has reorganized their netboot repository multiple times.

**Mitigation:** The asset manifest specifies the release, and the downloader uses the canonical URL pattern. If URLs change, only the downloader script needs updating (not the config or image).

**Status:** URLs should be verified at implementation time.

### 10.4 Large Initramfs Over HTTP

**Risk:** Talos initramfs is ~80MB (standard) or ~300MB (NVIDIA). Transferring this over HTTP from a Raspberry Pi could be slow.

**Mitigation:** Pi 4/5 has gigabit Ethernet and USB 3.0 — bandwidth is sufficient. For very large initramfs (NVIDIA), consider using the UKI boot method or compressing assets.

**Status:** Acceptable for homelab. Not a blocker.

### 10.5 UEFI iPXE User-Class Behavior

**Status:** Supported. Both BIOS and UEFI chainload paths are configured in dnsmasq.

**Note:** Most UEFI firmware correctly sends the iPXE user-class on re-DHCP
after loading `ipxe.efi`, which allows dnsmasq to distinguish stage 1 (TFTP)
from stage 2 (HTTP). Some older or buggy UEFI implementations may not send
the user-class, which would cause a chainload loop (getting `ipxe.efi` via
TFTP repeatedly instead of the HTTP URL). This is rare with modern firmware
but worth noting if a UEFI machine loops instead of booting.
