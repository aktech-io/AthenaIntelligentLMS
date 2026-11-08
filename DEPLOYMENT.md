# AthenaIntelligentLMS — k3s Deployment Guide

This document describes how to deploy the LMS microservices alongside the ADF (Athena Device Finance) platform using k3s (lightweight Kubernetes).

## Architecture Overview

Both ADF and LMS run on a single k3s node with namespace isolation:

| Namespace | Contents |
|-----------|----------|
| `infra` | PostgreSQL 16 (shared, 19 databases), RabbitMQ 3 |
| `adf` | 11 ADF backend services, 3 portals, API gateway |
| `lms` | 14 LMS microservices, LMS API gateway, LMS portal UI |
| `monitoring` | Prometheus, Grafana, Loki, log collector |

Eureka service discovery is **disabled** — all service-to-service communication uses Kubernetes DNS.

## LMS Services (namespace: `lms`)

| Service | Port | Database | K8s DNS |
|---------|------|----------|---------|
| account-service | 8086 | athena_accounts | `account-service.lms.svc.cluster.local:8086` |
| product-service | 8087 | athena_products | `product-service.lms.svc.cluster.local:8087` |
| loan-origination-service | 8088 | athena_loans | `loan-origination-service.lms.svc.cluster.local:8088` |
| loan-management-service | 8089 | athena_loans | `loan-management-service.lms.svc.cluster.local:8089` |
| payment-service | 8090 | athena_payments | `payment-service.lms.svc.cluster.local:8090` |
| accounting-service | 8091 | athena_accounting | `accounting-service.lms.svc.cluster.local:8091` |
| float-service | 8092 | athena_float | `float-service.lms.svc.cluster.local:8092` |
| collections-service | 8093 | athena_collections | `collections-service.lms.svc.cluster.local:8093` |
| compliance-service | 8094 | athena_compliance | `compliance-service.lms.svc.cluster.local:8094` |
| reporting-service | 8095 | athena_reporting | `reporting-service.lms.svc.cluster.local:8095` |
| ai-scoring-service | 8096 | athena_scoring | `ai-scoring-service.lms.svc.cluster.local:8096` |
| overdraft-service | 8097 | athena_overdraft | `overdraft-service.lms.svc.cluster.local:8097` |
| invoice-service | 8101 | athena_invoice | `invoice-service.lms.svc.cluster.local:8101` |
| media-service | 8098 | athena_media | `media-service.lms.svc.cluster.local:8098` |
| notification-service | 8099 | athena_notifications | `notification-service.lms.svc.cluster.local:8099` |
| lms-api-gateway | 8105 | — | `lms-api-gateway.lms.svc.cluster.local:8105` |
| lms-portal-ui | 3000 | — | `lms-portal-ui.lms.svc.cluster.local:3000` |

## Cross-Namespace Access

ADF accesses LMS via the `lms-gateway-service` in the ADF namespace, which proxies to LMS services:
- ADF API Gateway routes `/lms/**` to `lms-gateway-service.adf.svc.cluster.local:8100`
- `lms-gateway-service` forwards to individual LMS services in the `lms` namespace

Direct cross-namespace DNS example:
```
http://account-service.lms.svc.cluster.local:8086
```

## Shared Infrastructure

**PostgreSQL** (in `infra` namespace):
- Host: `postgres.infra.svc.cluster.local:5432`
- User: `admin` / Password: `password`
- All 19 databases initialized via ConfigMap-mounted init scripts
- `max_connections=300` to handle all microservices

**RabbitMQ** (in `infra` namespace):
- Host: `rabbitmq.infra.svc.cluster.local:5672`
- User: `guest` / Password: `guest`
- Management UI: port 15672

## Environment Variables (via ConfigMap `lms-common`)

All LMS services share these env vars:

```yaml
EUREKA_CLIENT_ENABLED: "false"
SPRING_CLOUD_DISCOVERY_ENABLED: "false"
SPRING_RABBITMQ_HOST: "rabbitmq.infra.svc.cluster.local"
SPRING_RABBITMQ_PORT: "5672"
SPRING_RABBITMQ_USERNAME: "guest"
SPRING_RABBITMQ_PASSWORD: "guest"
SPRING_DATASOURCE_USERNAME: "admin"
SPRING_DATASOURCE_PASSWORD: "password"
SPRING_JPA_HIBERNATE_DDL_AUTO: "none"
JWT_SECRET: "<configured in configmap>"
APP_ENV: "production"
LMS_INTERNAL_SERVICE_KEY: "<configured in configmap>"
```

Each service overrides `SPRING_DATASOURCE_URL` to point to its own database.

## Prerequisites

- k3s installed: `curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik" sh -`
- Docker (for building images)
- `kubectl` configured (`~/.kube/config` from `/etc/rancher/k3s/k3s.yaml`)

## Build LMS Images

All k8s scripts are in the ADF project: `/home/adira/AntigravityProjects/athena-device-finance/k8s/scripts/`

```bash
# Build all LMS images and import to k3s containerd
cd /home/adira/AntigravityProjects/athena-device-finance
./k8s/scripts/build-lms.sh

# Build a single service
./k8s/scripts/build-lms.sh account-service
```

Images are tagged as `lms/<service-name>:latest` and imported into k3s containerd via `docker save | sudo k3s ctr images import -`.

All deployments use `imagePullPolicy: Never` since images are local.

## Deploy

```bash
# Deploy everything (infra → adf → lms → monitoring)
./k8s/scripts/apply-all.sh

# Or deploy just the LMS namespace
kubectl apply -f k8s/lms/

# Check pod status
kubectl get pods -n lms

# View logs
kubectl -n lms logs -f deployment/account-service

# Restart a service
kubectl -n lms rollout restart deployment/account-service
```

## Access Points

| Service | URL |
|---------|-----|
| ADF API Gateway | http://localhost:30080 |
| ADF Admin Portal | http://localhost:30400 |
| Grafana | http://localhost:30300 |

LMS services are accessible internally via ClusterIP only. External access is through the ADF API Gateway (`/lms/**` routes).

## Teardown

```bash
# Remove all namespaces
./k8s/scripts/teardown.sh

# Or remove just LMS
kubectl delete namespace lms
```

## k8s Manifest Files

All manifests live in the ADF project under `k8s/`:

```
k8s/
  lms/
    namespace.yaml      — LMS namespace definition
    configmap.yaml      — Shared env vars (lms-common ConfigMap)
    services.yaml       — All 14 LMS microservice Deployments + ClusterIP Services
    gateway-portal.yaml — LMS API Gateway + Portal UI Deployments + Services
  infra/
    namespace.yaml
    postgres.yaml           — PostgreSQL StatefulSet + headless Service
    postgres-configmap.yaml — DB init scripts (creates all 19 databases)
    rabbitmq.yaml           — RabbitMQ StatefulSet + headless Service
  scripts/
    build-lms.sh    — Build all LMS Docker images
    build-adf.sh    — Build all ADF Docker images
    apply-all.sh    — Deploy all namespaces in order
    teardown.sh     — Remove all namespaces
```

## Troubleshooting

**Service won't start (CrashLoopBackOff)**:
```bash
kubectl -n lms logs <pod-name> --tail=50
```
Common causes: missing env vars (JWT_SECRET), DB not ready, port conflicts.

**Too many DB connections**:
PostgreSQL is configured with `max_connections=300`. If exceeded, check for duplicate pods:
```bash
kubectl -n lms get pods
# Clean up by scaling: kubectl -n lms scale deploy/<name> --replicas=0 && kubectl -n lms scale deploy/<name> --replicas=1
```

**Rebuild after code changes**:
```bash
./k8s/scripts/build-lms.sh <service-name>
kubectl -n lms rollout restart deployment/<service-name>
```

---

## Local Development (Docker Compose)

The Go services can be run locally alongside the ADF base infrastructure using Docker Compose overlays.

### Prerequisites

- Docker & Docker Compose v2
- Node.js 18+ (for portal UI dev server)
- The `AthenaCreditScore` repo cloned alongside this repo at `~/AthenaCreditScore`

### Quick Start

```bash
# 1. Create the shared Docker network (one-time)
docker network create athena-net

# 2. Start base infrastructure + all 16 Go services
cd ~/AthenaCreditScore
docker compose -f docker-compose.yml \
  -f ~/AthenaIntelligentLMS/docker-compose.go.yml up -d --build

# 3. Start the portal UI dev server
cd ~/AthenaIntelligentLMS/lms-portal-ui
npm install
npx vite --host 0.0.0.0 --port 3001
```

### Service Port Mapping (Docker Compose)

Go services run on `28xxx` host ports (internal port unchanged):

| Service | Internal Port | Host Port | Database |
|---------|--------------|-----------|----------|
| account-service | 8086 | 28086 | athena_accounts |
| product-service | 8087 | 28087 | athena_products |
| loan-origination-service | 8088 | 28088 | athena_loans |
| loan-management-service | 8089 | 28089 | athena_loans |
| payment-service | 8090 | 28090 | athena_payments |
| accounting-service | 8091 | 28091 | athena_accounting |
| float-service | 8092 | 28092 | athena_float |
| collections-service | 8093 | 28093 | athena_collections |
| compliance-service | 8094 | 28094 | athena_compliance |
| reporting-service | 8095 | 28095 | athena_reporting |
| ai-scoring-service | 8096 | 28096 | athena_scoring |
| overdraft-service | 8097 | 28097 | athena_overdraft |
| invoice-service | 8101 | 28103 | athena_invoice |
| media-service | 8098 | 28098 | athena_media |
| notification-service | 8099 | 28099 | athena_notifications |
| fraud-detection-service | 8100 | 28100 | athena_fraud |
| lms-api-gateway | 8105 | 28105 | — |
| portal-ui (Vite) | 3001 | 3001 | — |

### Test Accounts

All accounts use in-memory authentication (no database required):

| Username | Password | Role | Description |
|----------|----------|------|-------------|
| `admin` | `admin123` | ADMIN | System Administrator — full access |
| `admin@athena.com` | `admin123` | ADMIN | Same as above (email login) |
| `manager` | `manager123` | MANAGER | Branch Manager |
| `officer` | `officer123` | OFFICER | Loan Officer |
| `teller@athena.com` | `teller123` | TELLER | Senior Teller |

Passwords can be overridden via environment variables: `LMS_AUTH_ADMIN_PASSWORD`, `LMS_AUTH_MANAGER_PASSWORD`, `LMS_AUTH_OFFICER_PASSWORD`.

### Populating Test Data

```bash
# Populate 100 customers with full lifecycle data (customers, accounts, loans,
# payments, wallets, overdrafts, KYC, compliance alerts, media uploads)
python3 scripts/populate-300-customers.py --count 100
```

The script targets `28xxx` ports (Go services). It creates data across all services:
customers, savings accounts, loan applications through full lifecycle (submit → review → approve → disburse), repayments, wallets, overdraft facilities, KYC records, compliance alerts, payment records, and document uploads.

### Vite Proxy Configuration

The portal UI (`lms-portal-ui/vite.config.ts`) proxies API calls to Go services:

| Frontend Path | Backend Target |
|---------------|---------------|
| `/proxy/auth/*` | `localhost:28086` (account-service) |
| `/proxy/products/*` | `localhost:28087` (product-service) |
| `/proxy/loan-applications/*` | `localhost:28088` (loan-origination) |
| `/proxy/loans/*` | `localhost:28089` (loan-management) |
| `/proxy/payments/*` | `localhost:28090` (payment-service) |
| `/proxy/accounting/*` | `localhost:28091` (accounting-service) |
| `/proxy/float/*` | `localhost:28092` (float-service) |
| `/proxy/collections/*` | `localhost:28093` (collections-service) |
| `/proxy/compliance/*` | `localhost:28094` (compliance-service) |
| `/proxy/reporting/*` | `localhost:28095` (reporting-service) |
| `/proxy/scoring/*` | `localhost:28096` (ai-scoring-service) |
| `/proxy/fraud/*` | `localhost:28100` (fraud-detection) |

### Credential Configuration

The `docker-compose.go.yml` uses these defaults (matching the base `docker-compose.yml`):

| Variable | Default |
|----------|---------|
| `POSTGRES_USER` | `athena` |
| `POSTGRES_PASSWORD` | `athena_secret` |
| `RABBITMQ_USER` | `athena` |
| `RABBITMQ_PASS` | `athena_secret` |
| `JWT_SECRET` | Set via `.env` file |

### Checking Service Health

```bash
# Quick health check for all Go services
for port in 28086 28087 28088 28089 28090 28091 28092 28093 28094 28095 28096 28097 28098 28099 28100 28105; do
  status=$(curl -sf -o /dev/null -w "%{http_code}" http://localhost:$port/actuator/health 2>/dev/null || echo "DOWN")
  echo "  :$port → $status"
done
```
