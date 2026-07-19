# Contabo production box — lms namespace

Box: `ssh deploy@158.220.112.84` (k3s single node; API :6443 firewalled, SSH only;
`sudo k3s kubectl`). The `lms` namespace runs the raw-manifest deployment of the
Nemo/LMS platform — historical service names (`account-service`, …), NOT the Helm
chart's `go-*` names. Shared infra lives elsewhere: PostgreSQL in `alpa`
(reached via `postgres.infra.svc.cluster.local` ExternalName), RabbitMQ in
`infra`. Portal: https://lms.athenafinance.cloud.

## Files
- `gen-manifests.py` — single source of truth; emits `lms-nemo.yaml` for a tag.
- `lms-nemo.yaml` — generated manifest set (25 Deployments + Services).
- `lms-secrets.yaml` — **gitignored**; rotated credentials. Regenerate values per
  `../lms-secrets.example.yaml`; retrieve current ones from the cluster:
  `sudo k3s kubectl -n lms get secret lms-secrets -o yaml`.
- `create-databases.sql` — idempotent-ish DB creation for the shared PG
  (run in the `alpa` postgres pod; skips errors on existing DBs).

## Deploy — CI (the normal path, since 2026-07-20)
Push to `master` → `.github/workflows/deploy.yml` (mirrors alpa-api's pipeline)
builds the 25-image set to `ghcr.io/aktech-io/nemo-*:<sha>`, then over SSH
refreshes the `ghcr-pull` secret, runs `create-databases.sql` (idempotent),
applies `gen-manifests.py <sha> ghcr.io/aktech-io` output and waits for rollout.
Repo secrets: `DEPLOY_SSH_KEY` (dedicated CI key in `deploy`'s authorized_keys),
`CONTABO_HOST`; optional `GHCR_PULL_TOKEN` (fine-grained PAT, read:packages) to
make image pulls durable beyond the job-token lifetime — without it, pulls after
image GC need a re-run of the deploy job. One-time box prep (done 2026-07-20):
`lms-secrets` applied, `ROUTE_CARD_SERVICE_URL` in the ConfigMap, new DBs.

## Upgrade procedure — manual fallback (proven 2026-07-18)
1. Build: `TAG=<tag> ./scripts/build-nemo-images.sh` (needs dockerd running).
2. Ship: `docker save -o nemo-<tag>.tar <all 25 image refs>` → `scp` to the box
   (**never** stream through ssh — streamed import drops layers) →
   `sudo k3s ctr images import nemo-<tag>.tar`.
3. Databases: run `create-databases.sql` in the postgres pod
   (`sudo k3s kubectl -n alpa exec -i deploy/postgres -- psql -U admin -d postgres`).
4. Secrets: `sudo k3s kubectl apply -f lms-secrets.yaml` (first time or rotation).
   Rotating JWT_SECRET/LMS_INTERNAL_SERVICE_KEY requires rolling ALL services
   together (they must agree).
5. ConfigMap `lms-go-common`: keep only non-secret config — JWT_SECRET and
   LMS_INTERNAL_SERVICE_KEY are provided by `lms-secrets` (listed after the
   ConfigMap in envFrom, so the Secret wins on duplicate keys; delete the keys
   from the ConfigMap once all workloads carry the secretRef). Routes: ensure
   `ROUTE_CARD_SERVICE_URL=http://card-service:8107`.
6. Apply: `python3 gen-manifests.py <tag> > lms-nemo.yaml`, scp + 
   `sudo k3s kubectl apply -f lms-nemo.yaml`, then
   `sudo k3s kubectl -n lms rollout status deploy --timeout=300s`.
7. Verify: health sweep + full pytest suite over the SSH tunnel with the rotated
   password env (`LMS_ADMIN_PASSWORD=…` etc., see `tests/conftest.py`).

## Security posture (2026-07-18 upgrade)
- CRIT-2 closed on this box: `LMS_AUTH_ALLOW_DEFAULT_PASSWORDS` is gone;
  admin/manager/officer passwords come from `lms-secrets` (admin123 is dead).
- JWT_SECRET + LMS_INTERNAL_SERVICE_KEY rotated (old values were burned in git).
- Still open, deliberately deferred: DB password (`admin/password`) and RabbitMQ
  (`guest/guest`) are shared-infra credentials used by other namespaces
  (alpa/recon) — rotating them is a cross-namespace change; they also still sit
  in the `lms-go-common` ConfigMap. NetworkPolicy/HA hardening
  (`../hardening.yaml`) not applied — single-node box.
