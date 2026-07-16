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

Map the fields to `vault.yml`:

| talhelper field | vault.yml variable |
|----------------|-------------------|
| `machine.token` | `machine_token` |
| `certs.os.crt` | `machine_ca_cert` |
| `certs.os.key` | `machine_ca_key` |
| `cluster.id` | `cluster_id` |
| `cluster.secret` | `cluster_secret` |
| `secrets.bootstraptoken` | `cluster_token` |
| `secrets.secretboxencryptionsecret` | `secretbox_secret` |
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

## Encrypting vault.yml

After filling in real values:

```bash
cd ansible
ansible-vault encrypt group_vars/vault.yml
```

Store your vault password securely:

```bash
echo "your-vault-password" > ~/.vault_pass
chmod 600 ~/.vault_pass
```

Deploy with:
```bash
ansible-playbook site.yml -i inventory.ini --vault-password-file ~/.vault_pass
```

## Security Notes

- The `vault.yml` file in the repo has placeholder values only
- Gitleaks scans every commit for secrets
- Never commit real certs, keys, or tokens
- Use network segmentation (isolated VLAN) as your primary security control
- Apply the bootstrap node's config (with CA private key) out-of-band if possible
