# Siren QuickStart (Helm on Kubernetes)

This guide shows how to deploy **Siren** (Sigma Prime) with the chart under `./siren`
and how to supply the required secrets using small, generic helpers.

---

## Requirements

- Kubernetes **1.26+**, Helm **3.13+**
- Namespace (examples use `eth-validator`)
- Two K8s **Secrets**:
  - API token → key **`apitoken`** (used as `API_TOKEN`)
  - Session password → key **`password`** (used as `SESSION_PASSWORD`)

> Any helper can create these secrets. Below are generic patterns using
> portable scripts (`genpw.py`, `create_secret.py`) that can live either in
> this repo’s `tools/` or in a future `kubernetes/` subdirectory upstreamed
> to the Siren project.

---

## 1) Namespace

```bash
kubectl create namespace eth-validator
```

## 2) Create the secrets

**Session password (generate + create Secret):**
```bash
python tools/genpw.py -l 24   | python tools/create_secret.py -n eth-validator -s siren-session --key password
```

**API token (extract from Lighthouse validator + create Secret):**
```bash
# Path may vary across images; consult your validator docs.
kubectl -n eth-validator exec deploy/lighthouse-validator --   sh -lc 'cat "$HOME/.lighthouse/validators/api-token.txt"'   | python tools/create_secret.py -n eth-validator -s siren-api --key apitoken
```

> `create_secret.py` is useful here because it accepts **stdin**, so you can
> pipe the token directly from `kubectl exec` into the Secret with the exact
> key Siren expects.

## 3) Values

Create `values/siren.yaml`:

```yaml
image:
  repository: sigp/siren
  tag: "v3.0.4"   # pin to a tested version

service:
  type: ClusterIP
  port: 3000

config:
  beaconUrl: "http://<beacon-service>:<port>"       # -> BEACON_URL
  validatorUrl: "http://<validator-service>:<port>" # -> VALIDATOR_URL
  debug: false
  apiTokenSecretName: "siren-api"       # Secret with key: apitoken
  passwordSecretName: "siren-session"   # Secret with key: password
```

## 4) Install / Upgrade

```bash
helm upgrade --install siren ./siren -n eth-validator -f values/siren.yaml
kubectl -n eth-validator rollout status deploy/siren
```

## 5) Access the UI

```bash
kubectl -n eth-validator port-forward svc/siren 3000:3000
# http://localhost:3000
```

For shared access, enable and configure `ingress` in your values file.

## 6) Troubleshooting

- **Auth failures**: Confirm both secrets exist; keys are **exact** (`apitoken` / `password`).
- **No UI on 3000**: Ensure `service.port: 3000` and that you forwarded `3000:3000`.
- **Connectivity**: `beaconUrl`/`validatorUrl` must be reachable from the Siren pod.
