# Go-Live Hardening — Kubernetes

Production remediation for the findings in `docs/GO_LIVE_HARDENING_AUDIT.md`.
Apply to the **production** cluster; the local dev k3s intentionally keeps demo
config.

## Files
- `lms-secrets.example.yaml` — Secret template (CRIT-3). Copy → `lms-secrets.yaml`
  (gitignored), fill with generated values, apply.
- `hardening.yaml` — per-service liveness probe, replicas≥2, PDB, HPA, and a
  default-deny NetworkPolicy (HIGH availability gaps + CRIT-1 defence-in-depth).

## Apply order (production)
1. **Secrets:** generate strong values, `kubectl apply -f lms-secrets.yaml`.
2. **Move secret keys out of the ConfigMap** `lms-go-common` (keep only non-secret
   config) and add `envFrom: secretRef: lms-secrets` to every Deployment.
3. **Rotate** `LMS_INTERNAL_SERVICE_KEY` and `JWT_SECRET` (old values are burned —
   the service key is committed in git history; the JWT secret is a known
   placeholder). Roll all services together so they agree.
4. **Disable dev defaults:** ensure `LMS_AUTH_ALLOW_DEFAULT_PASSWORDS` is **unset**
   in prod and `LMS_AUTH_*_PASSWORD` come from the Secret (CRIT-2). account-service
   now fails to start rather than fall back to `admin/admin123`.
5. **Availability:** apply `hardening.yaml` per service (replicas≥2 + liveness +
   PDB + HPA). Requires metrics-server for HPA.
6. **Network:** expose ONLY the gateway via Ingress/LoadBalancer; apply the
   NetworkPolicy so backends are not internet-facing and the service-key path is
   in-cluster only.

## Status of code-level fixes (already shipped to `master`)
- CRIT-1 (gateway auth bypass): **fixed** — gateway rejects `X-Service-Key` and
  strips `X-Service-*` before proxying; verified forged key → 401.
- CRIT-2 (default creds): **fixed** — no hardcoded passwords; env/Secret-sourced.
- HIGH-1 (CORS): **fixed** — origin allowlist (`LMS_CORS_ALLOWED_ORIGINS`),
  credentials only for allowed origins.
- HIGH-2 (JWT fail-open): **fixed** — empty/short secret rejected.

The items in this directory (CRIT-3 secrets, HA, NetworkPolicy) are **infra/ops**
steps that require the production cluster and deliberate rotation — they are not
applied to the dev cluster automatically.
