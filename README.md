# PXE-in-a-Box: network boot service for Talos clusters
# https://github.com/homelab/pxe-in-a-box

A self-contained PXE boot server for homelab Talos clusters. Machines boot
from network on an empty drive, self-configure, and join an existing cluster
with zero operator interaction. Unknown machines get a boot menu.

## Quick Start

1. Copy `examples/` configs to `ansible/files/config/` and edit them
2. Extract Talos secrets from your existing cluster, put in Vault:
   ```
   ansible-vault edit ansible/group_vars/vault.yml
   ```
3. Deploy:
   ```
   cd ansible
   ansible-playbook site.yml -i inventory.ini --ask-vault-pass
   ```

## What It Does

- **Proxy DHCP + TFTP**: Chainloads PXE clients to iPXE via dnsmasq
- **MAC matching**: Known machines boot directly to their Talos profile
- **Boot menu**: Unknown machines get a menu (Talos maintenance / Ubuntu)
- **Config templating**: Parameterized Talos configs from `machines.yaml`
- **Asset management**: Auto-downloads kernels and initramfs

## Components

| Component | Purpose |
|-----------|---------|
| `pxe-gen` | Runs on Ansible controller, generates matchbox configs |
| `pxe-in-a-box` | Container entrypoint, manages dnsmasq + matchbox |
| `dnsmasq` | Proxy DHCP + TFTP server |
| `matchbox` | HTTP server for iPXE scripts, kernels, configs |

## Configuration Files

| File | Purpose |
|------|---------|
| `machines.yaml` | MAC → profile + template mapping |
| `assets.yaml` | What kernels/initramfs to download |
| `menu.yaml` | Boot menu for unknown machines |
| `templates/*.j2` | Talos machine config templates |

See `REQUIREMENTS.md` and `ARCHITECTURE.md` for full documentation.

## Network Requirements

- **DHCP**: Your existing DHCP server (e.g., UDM Pro) handles IP assignment
- **TFTP**: UDM Pro points clients to this server for TFTP
- **Network**: PXE host needs a static IP (DHCP reservation)
- **BIOS only**: UEFI support is planned for a future release
