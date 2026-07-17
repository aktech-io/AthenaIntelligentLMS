# Nemo umbrella chart (D1)

One-command install of the whole platform: 16 Go services + API gateway,
fraud-ML sidecar, in-cluster PostgreSQL (all service databases auto-created on
first boot) and RabbitMQ, wired together with the same env contract as the
compose/k3s deployments and configured by market pack (`global.marketPack`).

```bash
# Demo/dev (single node, demo credentials, local images):
helm install nemo deploy/helm/nemo

# Production shape:
helm install nemo deploy/helm/nemo \
  --set global.imageRegistry=registry.example.com/nemo \
  --set global.existingSecret=nemo-prod-credentials \
  --set gateway.ingress.enabled=true --set gateway.ingress.host=api.bank.example \
  --set db.host=managed-postgres.internal --set postgresql.enabled=false \
  --set rabbitmq.host=managed-rabbit.internal --set rabbitmq.enabled=false
```

Images are expected as `nemo-<service>:<tag>` (prefixed by `global.imageRegistry`
when set); build them from `go-services/deploy/docker/Dockerfile.service` with
`--build-arg SERVICE=<name>`.

Production hardening (replicas≥2, PDB, HPA, NetworkPolicy, secret rotation) —
see `deploy/k8s/README.md`; those pieces fold into this chart as D3/D4 land.

Still to come on the D-track: image build/publish pipeline, offline bundle for
air-gapped installs (D2), migration gating for zero-downtime upgrades (D3),
HA/DR topology values (D4), and the observability stack in-chart (H1).
