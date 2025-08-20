# eth-validator

Deploy an Ethereum validator on Kubernetes

## TL;DR
```bash
helm upgrade --install eth-validator ./eth-validator -n eth-validator -f values.yaml
```

## Prerequisites
- Kubernetes >= 1.27
- A default `StorageClass`
- (Optional) A LoadBalancer solution (e.g., MetalLB or cloud LB) if exposing services externally

## Example values (annotated, abridged)

> Use this as a starting point. Anything in `<angle-brackets>` must be customized.
> See the full example at: `docs/examples/eth-validator-values-example.yaml`

```yaml
geth:
  image:
    repository: ethereum/client-go
    tag: "v1.16.2"
    pullPolicy: IfNotPresent
  resources:
    requests: { cpu: 3, memory: 12Gi }
    limits:   { cpu: 3, memory: 12Gi }
  cache: 8192
  maxPeers: 40
  nodeSelector:
    kubernetes.io/hostname: "<node-for-geth>"
  storage:
    class: "<storage-class>"
    size: 100Gi
  external:
    enabled: true
    type: LoadBalancer
    port: 30503
    annotations:
      metallb.universe.tf/loadBalancerIPs: "<lb-ip-for-geth>"

lighthouseBeacon:
  image:
    repository: sigp/lighthouse
    tag: "v7.1.0"
    pullPolicy: IfNotPresent
  checkpointSyncUrl: "https://<checkpoint-endpoint>/"
  gui: true
  targetPeers: 60
  timeoutMultiplier: 3
  nodeSelector:
    kubernetes.io/hostname: "<node-for-beacon>"
  resources:
    requests: { cpu: 4, memory: 16Gi }
    limits:   { cpu: 4, memory: 16Gi }
  storage:
    class: "<storage-class>"
    size: 100Gi
  external:
    enabled: true
    type: LoadBalancer
    port: 31600
    annotations:
      metallb.universe.tf/loadBalancerIPs: "<lb-ip-for-beacon>"
  mev:
    enabled: true
    image: { repository: flashbots/mev-boost, tag: "1.9", pullPolicy: Always }
    relays:
      - "https://<relay-pubkey>@<relay-host>"

lighthouseValidator:
  image:
    repository: sigp/lighthouse
    tag: "v1.2.0"
    pullPolicy: Always
  loglevel: "debug"
  nodeSelector:
    kubernetes.io/hostname: "<node-for-validator>"
  storage:
    class: "<storage-class>"
```

### Where you'll customize

- **Network bootstrap** — `lighthouseBeacon.checkpointSyncUrl` must match your network (e.g., hoodi uses `https://hoodi.beaconstate.ethstaker.cc/`).
- **Relays** — `lighthouseBeacon.mev.relays` must be valid for your network/policy.
- **Exposure** — `*.external.*` (LoadBalancer ports, annotations, and IPs) depend on whether you use MetalLB, a cloud LB, or keep services internal.
- **Placement** — `nodeSelector` targets; pick nodes with the right CPU/disk (EL on fastest disk).
- **Storage** — `storage.class` and `size`. For local dev: `local-path`. On cloud: `gp3`, `pd-ssd`, etc.
- **Resources** — `requests/limits` should match your cluster capacity and performance goals.
- **Versions** — keep `image.tag` pinned; upgrade deliberately and roll back with Helm history if needed.
- **Optional fields** — `externalIp` is provided for charts that consume it; remove if unused.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| autoscaling.enabled | bool | `false` |  |
| autoscaling.maxReplicas | int | `100` |  |
| autoscaling.minReplicas | int | `1` |  |
| autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| externalIp | string | `""` |  |
| fullnameOverride | string | `""` |  |
| geth.affinity | object | `{}` |  |
| geth.cache | int | `4096` |  |
| geth.disableHistory | bool | `true` |  |
| geth.external.annotations | object | `{}` |  |
| geth.external.p2p.port | int | `30303` |  |
| geth.external.type | string | `"LoadBalancer"` |  |
| geth.image.pullPolicy | string | `"IfNotPresent"` |  |
| geth.image.repository | string | `"ethereum/client-go"` |  |
| geth.image.tag | string | `"latest"` |  |
| geth.imagePullSecrets | list | `[]` |  |
| geth.internal.annotations | object | `{}` |  |
| geth.internal.api.port | int | `8545` |  |
| geth.internal.auth.port | int | `8551` |  |
| geth.internal.metrics.port | int | `6060` |  |
| geth.internal.type | string | `"ClusterIP"` |  |
| geth.maxPeers | int | `0` |  |
| geth.nodeSelector | object | `{}` |  |
| geth.podAnnotations | object | `{}` |  |
| geth.podSecurityContext | object | `{}` |  |
| geth.replicaCount | int | `1` |  |
| geth.resources.limits.cpu | int | `3` |  |
| geth.resources.limits.memory | string | `"10G"` |  |
| geth.resources.requests.cpu | int | `3` |  |
| geth.resources.requests.memory | string | `"10G"` |  |
| geth.securityContext | object | `{}` |  |
| geth.storage.class | string | `"default"` |  |
| geth.storage.size | string | `"600G"` |  |
| ingress.annotations | object | `{}` |  |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| ingress.hosts[0].host | string | `"chart-example.local"` |  |
| ingress.hosts[0].paths[0].path | string | `"/"` |  |
| ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"` |  |
| ingress.tls | list | `[]` |  |
| lighthouseBeacon.affinity | object | `{}` |  |
| lighthouseBeacon.asyncAPI | bool | `true` |  |
| lighthouseBeacon.checkpointSyncUrl | string | `""` |  |
| lighthouseBeacon.external.annotations | object | `{}` |  |
| lighthouseBeacon.external.p2p.port | int | `31400` |  |
| lighthouseBeacon.external.type | string | `"LoadBalancer"` |  |
| lighthouseBeacon.gui | bool | `false` |  |
| lighthouseBeacon.image.pullPolicy | string | `"IfNotPresent"` |  |
| lighthouseBeacon.image.repository | string | `"sigp/lighthouse"` |  |
| lighthouseBeacon.image.tag | string | `"latest"` |  |
| lighthouseBeacon.imagePullSecrets | list | `[]` |  |
| lighthouseBeacon.internal.annotations | object | `{}` |  |
| lighthouseBeacon.internal.api.port | int | `5052` |  |
| lighthouseBeacon.internal.metrics.port | int | `5054` |  |
| lighthouseBeacon.internal.type | string | `"ClusterIP"` |  |
| lighthouseBeacon.mev.enabled | bool | `true` |  |
| lighthouseBeacon.mev.image.pullPolicy | string | `"IfNotPresent"` |  |
| lighthouseBeacon.mev.image.repository | string | `"flashbots/mev-boost"` |  |
| lighthouseBeacon.mev.image.tag | string | `"latest"` |  |
| lighthouseBeacon.mev.imagePullSecrets | list | `[]` |  |
| lighthouseBeacon.mev.port | int | `18550` |  |
| lighthouseBeacon.mev.relays | list | `[]` |  |
| lighthouseBeacon.mev.securityContext | object | `{}` |  |
| lighthouseBeacon.nodeSelector | object | `{}` |  |
| lighthouseBeacon.podAnnotations | object | `{}` |  |
| lighthouseBeacon.podSecurityContext | object | `{}` |  |
| lighthouseBeacon.rayonThreads | int | `0` |  |
| lighthouseBeacon.replicaCount | int | `1` |  |
| lighthouseBeacon.resources.limits.cpu | int | `4` |  |
| lighthouseBeacon.resources.limits.memory | string | `"16G"` |  |
| lighthouseBeacon.resources.requests.cpu | int | `4` |  |
| lighthouseBeacon.resources.requests.memory | string | `"16G"` |  |
| lighthouseBeacon.securityContext | object | `{}` |  |
| lighthouseBeacon.stateCacheSize | int | `0` |  |
| lighthouseBeacon.storage.class | string | `"default"` |  |
| lighthouseBeacon.storage.size | string | `"250G"` |  |
| lighthouseBeacon.targetPeers | int | `0` |  |
| lighthouseBeacon.timeoutMultiplier | int | `0` |  |
| lighthouseValidator.affinity | object | `{}` |  |
| lighthouseValidator.image.pullPolicy | string | `"IfNotPresent"` |  |
| lighthouseValidator.image.repository | string | `"wateim/lighthouse-launch"` |  |
| lighthouseValidator.image.tag | string | `"latest"` |  |
| lighthouseValidator.imagePullSecrets | list | `[]` |  |
| lighthouseValidator.internal.annotations | object | `{}` |  |
| lighthouseValidator.internal.api.port | int | `5052` |  |
| lighthouseValidator.internal.launch.port | int | `5000` |  |
| lighthouseValidator.internal.metrics.port | int | `5054` |  |
| lighthouseValidator.internal.type | string | `"ClusterIP"` |  |
| lighthouseValidator.loglevel | string | `""` |  |
| lighthouseValidator.nodeSelector | object | `{}` |  |
| lighthouseValidator.podAnnotations | object | `{}` |  |
| lighthouseValidator.podSecurityContext | object | `{}` |  |
| lighthouseValidator.replicaCount | int | `1` |  |
| lighthouseValidator.resources.limits.cpu | int | `1` |  |
| lighthouseValidator.resources.limits.memory | string | `"2G"` |  |
| lighthouseValidator.resources.requests.cpu | int | `1` |  |
| lighthouseValidator.resources.requests.memory | string | `"2G"` |  |
| lighthouseValidator.secretsDir.mountPath | string | `"/secrets"` |  |
| lighthouseValidator.secretsDir.useFlag | bool | `true` |  |
| lighthouseValidator.securityContext | object | `{}` |  |
| lighthouseValidator.storage.class | string | `"default"` |  |
| lighthouseValidator.storage.size | string | `"2G"` |  |
| nameOverride | string | `""` |  |
| network | string | `"hoodi"` |  |
| nodeSelector | object | `{}` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| tolerations | list | `[]` |  |

## Notes
- This chart does **not** set defaults for external service URLs or network endpoints; provide them in your values.
- Secrets (fee recipient, graffiti, keystores, JWT, etc.) are chart-specific; see the chart values for the exact keys.
