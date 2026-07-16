# Operations Guide

## Day-to-Day Operations

### Adding a New Machine

1. Get the machine's MAC address
2. Add it to `machines.yaml` under the appropriate group:

```yaml
groups:
  - name: workers
    ...
    machines:
      - mac: aa:bb:cc:dd:00:13
        hostname: worker-03
        vars:
          node_ip: 192.168.1.23
          install_disk_selector: naa.YOUR_DISK_WWID
```

3. Run the playbook:
```bash
ansible-playbook site.yml -i inventory.ini --ask-vault-pass
```

4. Plug in the machine, set BIOS to PXE boot. It will auto-provision.

### Removing a Machine

1. Remove the entry from `machines.yaml`
2. Re-run the playbook
3. The stale group/profile files are automatically cleaned up

### Adding a New Talos Version

1. Add to `assets.yaml`:
```yaml
talos:
  - id: talos-v1.11.0
    version: v1.11.0
    arch: amd64
```

2. Re-run the playbook — the container auto-downloads the new kernel/initramfs
3. Update `machines.yaml` to point machines to the new version if desired

### Machine Recovery (Transient Failure)

Just power the machine back on. If it has Talos installed on disk, it boots
from disk normally. If the disk is wiped/empty, it PXE boots and reinstalls
automatically.

No intervention needed — the cycle is idempotent.

## Debugging

### Check what matchbox knows

```bash
docker exec pxe-in-a-box pxe-in-a-box --dump-state
```

### Verify HTTP endpoints

```bash
# Known MAC should return iPXE script
curl http://PXE_HOST:8081/ipxe?mac=YOUR:MAC:HERE

# Unknown MAC should return 404
curl -s -o /dev/null -w "%{http_code}" http://PXE_HOST:8081/ipxe?mac=aa:bb:cc:dd:ee:ff

# Boot script
curl http://PXE_HOST:8081/assets/boot.ipxe

# Machine config
curl http://PXE_HOST:8081/assets/rendered/YOUR_HOSTNAME.yaml
```

### Check container logs

```bash
docker logs pxe-in-a-box -f
```

Matchbox logs every HTTP request including the MAC address, so you can see
which machines are attempting to boot.

### Check dnsmasq DHCP/TFTP

```bash
docker logs pxe-in-a-box 2>&1 | grep dnsmasq
```

Look for DHCPDISCOVER, proxy DHCP responses, and TFTP transfers.

### Common Issues

**Machine loops at iPXE, never gets HTTP script:**
- Check dnsmasq `dhcp-userclass` is working — the machine may not be getting
  the iPXE HTTP redirect. Look for repeated TFTP transfers in logs.
- Ensure `pxe-service=tag:ipxe` line points to correct IP and port.

**Known MAC gets 404 instead of boot script:**
- Check the group JSON exists: `docker exec pxe-in-a-box ls /config/groups/`
- Verify the MAC matches (lowercase, colon-separated): matchbox normalizes
  hexhyp format (`aa-bb-cc-dd-ee-ff`) internally.

**Talos boots but can't fetch machine config:**
- Verify the `talos.config=` URL in the profile is reachable:
  `curl http://PXE_HOST:8081/assets/rendered/HOSTNAME.yaml`
- Check the rendered config file exists and has valid YAML.

**Kernel/initramfs 404:**
- Check assets downloaded: `docker exec pxe-in-a-box ls /assets/talos-*/amd64/`
- Check container startup logs for download failures.
- Verify network access to GitHub releases or Image Factory from the PXE host.

## Updating the PXE Container

```bash
# Pull new image
ansible-playbook site.yml -i inventory.ini --ask-vault-pass
```

The playbook pulls the latest image and restarts the container. Assets on
the mounted volume are preserved. Brief PXE interruption (seconds) during
restart is expected.

## Asset Cleanup

By default, old assets are NOT deleted. To enable cleanup:

In `assets.yaml`:
```yaml
cleanup: true
```

Or via CLI flag:
```bash
docker exec pxe-in-a-box pxe-in-a-box --cleanup
```

To preview what would be deleted:
```bash
docker exec pxe-in-a-box pxe-in-a-box --dry-run --cleanup
```

Protected paths (never deleted): `rendered/`, `static/`, `boot.ipxe`.
