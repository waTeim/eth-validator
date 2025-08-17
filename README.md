# k8ev-kit — Ethereum Validator Toolkit

[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.26%2B-blue)
![Helm](https://img.shields.io/badge/Helm-3.13%2B-blue)

`k8ev-kit` is a modular toolkit for deploying and operating an Ethereum validator
stack on Kubernetes (incl. OpenShift). It combines Helm charts, a Go launcher,
and small Python helpers to streamline day‑zero setup and day‑two ops.

---

## Repository Structure (develop)

```
k8ev-kit/
├─ eth-validator/          # Main validator chart + templates (cluster wiring)
│  ├─ Chart.yaml  values.yaml  templates/
├─ lighthouse-launch/      # Go-based launcher / API for Lighthouse workflows
│  ├─ Dockerfile  Makefile  main.go  swagger.yaml ...
├─ siren/                  # Helm chart for Sigma Prime's Siren (validator dashboard)
│  ├─ Chart.yaml  values.yaml  templates/
├─ tools/                  # Small Python helpers
│  ├─ genpw.py             # password generator (stdout; length, charset options)
│  └─ create_secret.py     # kubernetes Secret helper (stdin/file → Secret)
└─ README.md
```

### Why this split?
- **eth-validator/**: cluster plumbing and common patterns
- **lighthouse-launch/**: optional API/front‑end to manage Lighthouse validators
- **siren/**: first‑class chart to deploy the Siren dashboard alongside your stack
- **tools/**: thin, scriptable helpers you can use locally or in CI

---

## Quick Start

> Assumes a working `kubectl` context and a default StorageClass. Replace the
> namespace if you don’t use `eth-validator`.

```bash
# 1) Clone
git clone https://github.com/waTeim/k8ev-kit.git
cd k8ev-kit

# 2) Create namespace (once)
kubectl create namespace eth-validator
```

### Deploy Siren (dashboard)

Siren shows validator status/metrics. The chart expects two K8s secrets:

- **API token** secret → provides `API_TOKEN` (key must be `apitoken`)
- **Session password** secret → provides `SESSION_PASSWORD` (key must be `password`)

You can create these with any tool; here are **generic patterns** using the helpers:

```bash
# A) Generate a strong session password and store it as Secret key `password`
python tools/genpw.py -l 24   | python tools/create_secret.py -n eth-validator -s siren-session --key password

# B) Extract Lighthouse validator API token from the running validator pod
#    (path may vary; see your client docs). Example generic pattern:
kubectl -n eth-validator exec deploy/lighthouse-validator --   sh -lc 'cat "$HOME/.lighthouse/validators/api-token.txt"'   | python tools/create_secret.py -n eth-validator -s siren-api --key apitoken
```

> **Why `create_secret.py` matters:** it lets you pipe values from `kubectl exec`
> directly into a K8s Secret with the correct key names, which matches Siren’s
> environment mapping. This is convenient when following Siren’s own token
> extraction instructions from within Kubernetes.

Now prepare a minimal values file for Siren:

```bash
mkdir -p values
cat > values/siren.yaml <<'YAML'
image:
  repository: sigp/siren
  tag: "v3.0.4"   # pin a tested version

service:
  type: ClusterIP
  port: 3000

config:
  beaconUrl: "http://<beacon-service>:<port>"       # -> BEACON_URL
  validatorUrl: "http://<validator-service>:<port>" # -> VALIDATOR_URL
  debug: false
  apiTokenSecretName: "siren-api"       # Secret with key: apitoken
  passwordSecretName: "siren-session"   # Secret with key: password
YAML
```

Install Siren:
```bash
helm upgrade --install siren ./siren -n eth-validator -f values/siren.yaml
kubectl -n eth-validator rollout status deploy/siren
kubectl -n eth-validator port-forward svc/siren 3000:3000
# open http://localhost:3000
```

### Deploy the validator stack

This repo provides patterns for the broader stack under `eth-validator/` and
an optional API under `lighthouse-launch/`. Bring your preferred EL/CL/VC and
wire them up using your own values files.

---

## Ops & Troubleshooting

```bash
# Check pods
kubectl -n eth-validator get pods

# Tail Siren logs
kubectl -n eth-validator logs deploy/siren -f

# Helm history + rollback
helm -n eth-validator history siren
helm -n eth-validator rollback siren <REVISION>
```

Common gotchas:
- Secret keys must match **exactly**: `apitoken` for the API token, `password` for the session password.
- `beaconUrl` / `validatorUrl` must be reachable from the Siren pod (ClusterIP DNS is typical).
- If exposing Siren, set up `ingress.enabled: true` and supply host/TLS in your values file.

---

## Using the helpers generically

These helpers are intentionally generic so they can live either under this repo’s
`tools/` **or** be copied into a future `kubernetes/` subdirectory of the upstream
Siren project.

- `genpw.py`: prints a strong password to stdout (e.g., `-l 24` to set length). Pipe into whatever needs it.
- `create_secret.py`: reads from **stdin** (or a file), creates/updates a Secret with a chosen name and key.
  - Suggested flags: `--namespace/-n`, `--secretname/-s`, `--key <key>`
  - Example: `echo foo | create_secret.py -n ns -s my-secret --key somekey`

> If your local copy uses slightly different flags, adapt accordingly. The idea is
> the same: stream a value directly into the Secret key that downstream charts expect.

---

## Roadmap

- Add example values for Geth + Lighthouse
- Optional ServiceMonitor / Grafana dashboards
- OpenShift‑friendly presets (SCC / SecurityContext profiles)

---

## License

MIT © waTeim
