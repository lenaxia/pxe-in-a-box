# Secret Extraction Guide

PXE-in-a-Box never generates Talos secrets. You provide them from your
existing cluster. This guide covers extraction methods.

## Option 1: From talhelper (recommended)

If you manage your cluster with [talhelper](https://github.com/budimanjojo/talhelper):

```bash
cd your-talhelper-dir/kubernetes/bootstrap/talos

# Decrypt the secrets file
sops -d talsecret.sops.yaml > talsecret.yaml
```

Map the fields to `secrets.yaml`:

| talhelper field | secrets.yaml field |
|----------------|-------------------|
| `machine.token` | `machine_token` |
| `certs.os.crt` | `machine_ca_cert` |
| `certs.os.key` | `machine_ca_key` |
| `cluster.id` | `cluster_id` |
| `cluster.secret` | `cluster_secret` |
| `secrets.bootstraptoken` | `cluster_token` |
| `secrets.secretboxencryptionsecret` | `secretbox_key` |
| `certs.k8s.crt` | `cluster_ca_cert` |
| `certs.k8s.key` | `cluster_ca_key` |
| `certs.k8saggregator.crt` | `aggregator_ca_cert` |
| `certs.k8saggregator.key` | `aggregator_ca_key` |
| `certs.k8sserviceaccount.key` | `service_account_key` |
| `certs.etcd.crt` | `etcd_ca_cert` |
| `certs.etcd.key` | `etcd_ca_key` |

## Option 2: From a rendered machine config

Read values directly from any existing node's machine config:

```bash
# Get a node's config
talosctl -n NODE_IP read /system/state/config.yaml > node-config.yaml

# Extract values
grep "token:" node-config.yaml           # machine_token (first), cluster_token (second)
grep "  id:" node-config.yaml            # cluster_id
grep "  secret:" node-config.yaml        # cluster_secret
```

For control plane nodes, the full CA certs/keys are in the config. Worker
nodes have certs but no private keys.

## Option 3: From an existing matchbox/talos setup

If you have existing rendered configs (e.g., in a `clusterconfig/` directory):

```bash
python3 << 'EOF'
import yaml
with open("clusterconfig/home-kubernetes-cp-00.yaml") as f:
    cfg = list(yaml.safe_load_all(f))[0]
m, c = cfg["machine"], cfg["cluster"]
print(f"machine_token: {m['token']}")
print(f"cluster_token: {c['token']}")
print(f"cluster_id: {c['id']}")
print(f"cluster_secret: {c['secret']}")
print(f"cluster_endpoint: {c['controlPlane']['endpoint']}")
EOF
```

## Option 4: Automated extraction script

```bash
python3 << 'PYEOF'
import yaml

ORIG = "clusterconfig/home-kubernetes-cp-00.yaml"  # any CP node config

with open(ORIG) as f:
    cfg = list(yaml.safe_load_all(f))[0]

m = cfg["machine"]
c = cfg["cluster"]

lines = ["# Extracted cluster secrets for PXE-in-a-Box", "---"]
lines.append('machine_token: "%s"' % m["token"])
lines.append('machine_ca_cert: "%s"' % m["ca"]["crt"])
lines.append('machine_ca_key: "%s"' % m["ca"].get("key", ""))
lines.append('cluster_id: "%s"' % c["id"])
lines.append('cluster_secret: "%s"' % c["secret"])
lines.append('cluster_name: "%s"' % c["clusterName"])
lines.append('cluster_endpoint: "192.168.1.30"')  # set to your VIP
lines.append('cluster_token: "%s"' % c["token"])
lines.append('secretbox_key: "%s"' % c.get("secretboxEncryptionSecret", ""))
lines.append('cluster_ca_cert: "%s"' % c["ca"]["crt"])
lines.append('cluster_ca_key: "%s"' % c["ca"].get("key", ""))
agg = c.get("aggregatorCA", {})
lines.append('aggregator_ca_cert: "%s"' % agg.get("crt", ""))
lines.append('aggregator_ca_key: "%s"' % agg.get("key", ""))
sa = c.get("serviceAccount", {})
lines.append('service_account_key: "%s"' % sa.get("key", ""))
etcd = c.get("etcd", {}).get("ca", {})
lines.append('etcd_ca_cert: "%s"' % etcd.get("crt", ""))
lines.append('etcd_ca_key: "%s"' % etcd.get("key", ""))

with open("secrets.yaml", "w") as f:
    f.write("\n".join(lines) + "\n")

print("Wrote secrets.yaml")
PYEOF
```

## SOPS encryption (optional)

If you want secrets encrypted at rest:

```bash
# Generate an age key
age-keygen -o config/age.key

# Encrypt secrets
sops --encrypt --age $(grep -oP 'age1.*' config/age.key) \
     --in-place config/secrets.yaml
mv config/secrets.yaml config/secrets.sops.yaml
```

The container auto-detects `secrets.sops.yaml` + `age.key` and decrypts
at startup (requires `sops` binary in the container).

## Security Notes

- The `secrets.yaml` example in the repo has placeholder values only
- Gitleaks scans every commit for secrets
- Never commit real certs, keys, or tokens
- Use network segmentation (isolated VLAN) as your primary security control
- Apply the bootstrap node's config (with CA private key) out-of-band if possible
