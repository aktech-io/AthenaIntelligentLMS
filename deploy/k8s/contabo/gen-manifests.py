#!/usr/bin/env python3
"""Generate the Contabo lms-namespace manifest set (deploy/k8s/contabo/lms-nemo.yaml).

The Contabo box (158.220.112.84, k3s) runs the raw-manifest deployment of the
Nemo/LMS platform in namespace `lms` — service names are the box's historical
ones (account-service, ...), NOT the Helm chart's go-* names. This generator is
the single source of truth for that box; regenerate + `kubectl apply` to roll.

Usage:
  ./gen-manifests.py [TAG] [REGISTRY]   # default: latest, local images
  ./gen-manifests.py abc123 ghcr.io/aktech-io   # CI: registry-prefixed images
                                                # + imagePullSecrets ghcr-pull
"""
import sys

TAG = sys.argv[1] if len(sys.argv) > 1 else "latest"
REGISTRY = sys.argv[2] if len(sys.argv) > 2 else ""
NS = "lms"


def image_ref(name):
    ref = f"{name}:{TAG}"
    return f"{REGISTRY}/{ref}" if REGISTRY else ref


# registry pulls need the ghcr-pull dockerconfigjson secret (refreshed by CI)
PULL_SECRETS = ("      imagePullSecrets:\n      - name: ghcr-pull\n"
                if REGISTRY else "")

# name, port, db_name (None = no DB), extra env dict
GO_SERVICES = [
    ("account-service",          8086, "athena_accounts", {}),
    ("product-service",          8087, "athena_products", {}),
    ("loan-origination-service", 8088, "athena_loans", {}),
    ("loan-management-service",  8089, "athena_loans", {}),
    ("payment-service",          8090, "athena_payments", {}),
    ("accounting-service",       8091, "athena_accounting", {}),
    ("float-service",            8092, "athena_float", {}),
    ("collections-service",      8093, "athena_collections", {}),
    ("compliance-service",       8094, "athena_compliance", {
        "EKYC_PROVIDER": "inhouse",
        "EKYC_ML_SERVICE_URL": "http://nemo-ekyc-ml:8102",
        "MEDIA_SERVICE_URL": "http://media-service:8098",
    }),
    ("reporting-service",        8095, "athena_reporting", {}),
    ("ai-scoring-service",       8096, "athena_scoring", {}),
    ("overdraft-service",        8097, "athena_overdraft", {}),
    ("media-service",            8098, "athena_media", {}),
    ("notification-service",     8099, "athena_notifications", {}),
    ("fraud-detection-service",  8100, "athena_fraud", {
        "FRAUD_ML_SERVICE_URL": "http://nemo-fraud-ml:8101",
    }),
    ("lms-api-gateway",          8105, None, {
        "LMS_CORS_ALLOWED_ORIGINS": "https://lms.athenafinance.cloud,https://app.lms.athenafinance.cloud",
    }),
    ("decision-service",         8106, "athena_decision", {}),
    ("card-service",             8107, "athena_cards", {"CARD_PROCESSOR": "sandbox"}),
    # Mobile BFF — app-facing; call INTO the LMS services directly (the public
    # gateway strips X-Service-Key, so service-key calls must not transit it).
    ("bff-gateway",              8110, "athena_mobile_gateway", {
        "ACCOUNT_SERVICE_URL": "http://account-service:8086",
        "OVERDRAFT_SERVICE_URL": "http://overdraft-service:8097",
        "PAYMENT_SERVICE_URL": "http://payment-service:8090",
        "PRODUCT_SERVICE_URL": "http://product-service:8087",
        "LOAN_ORIGINATION_SERVICE_URL": "http://loan-origination-service:8088",
        "LOAN_MANAGEMENT_SERVICE_URL": "http://loan-management-service:8089",
        "AI_SCORING_SERVICE_URL": "http://ai-scoring-service:8096",
        "COMPLIANCE_SERVICE_URL": "http://compliance-service:8094",
        "MEDIA_SERVICE_URL": "http://media-service:8098",
        "NOTIFICATION_SERVICE_URL": "http://bff-notification:8111",
        "LMS_CORS_ALLOWED_ORIGINS": "https://app.lms.athenafinance.cloud",
    }),
    ("bff-notification",         8111, "athena_bff_notifications", {}),
    ("bff-billpay-savings",      8112, "athena_billpay_savings", {
        "ACCOUNT_SERVICE_URL": "http://account-service:8086",
        "PAYMENT_SERVICE_URL": "http://payment-service:8090",
    }),
    ("bff-shop",                 8113, "athena_shop", {
        "ACCOUNT_SERVICE_URL": "http://account-service:8086",
        "PAYMENT_SERVICE_URL": "http://payment-service:8090",
        "LOAN_ORIGINATION_SERVICE_URL": "http://loan-origination-service:8088",
        "AI_SCORING_SERVICE_URL": "http://ai-scoring-service:8096",
    }),
]

# Python ML sidecars: name, port, image, health path, extra env
ML_SERVICES = [
    # No MLflow on this box — fail model-registry lookups fast instead of
    # hanging scoring requests (rule-based fallback takes over).
    ("nemo-fraud-ml", 8101, image_ref("nemo-fraud-ml"), "/health",
     {"MLFLOW_HTTP_REQUEST_TIMEOUT": "3", "MLFLOW_HTTP_REQUEST_MAX_RETRIES": "1"}),
    ("nemo-ekyc-ml",  8102, image_ref("nemo-ekyc-ml"), "/health", {}),
]


def deployment(name, port, image, env, health_path, requests, limits,
               envfrom_secret=True, initial_delay=40):
    env_yaml = ""
    for k, v in env.items():
        env_yaml += f'        - name: {k}\n          value: "{v}"\n'
    envfrom = "        envFrom:\n        - configMapRef: {name: lms-go-common}\n"
    if envfrom_secret:
        # listed after the ConfigMap: duplicate keys resolve to the Secret
        envfrom += "        - secretRef: {name: lms-secrets}\n"
    return f"""---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {name}
  namespace: {NS}
  labels: {{app: {name}}}
spec:
  replicas: 1
  selector:
    matchLabels: {{app: {name}}}
  template:
    metadata:
      labels: {{app: {name}}}
    spec:
{PULL_SECRETS}      containers:
      - name: {name}
        image: {image}
        imagePullPolicy: IfNotPresent
{envfrom.rstrip()}
        env:
{env_yaml.rstrip()}
        ports:
        - containerPort: {port}
        readinessProbe:
          httpGet: {{path: {health_path}, port: {port}}}
          initialDelaySeconds: {initial_delay}
          periodSeconds: 10
          timeoutSeconds: 3
        livenessProbe:
          httpGet: {{path: {health_path}, port: {port}}}
          initialDelaySeconds: {initial_delay + 20}
          periodSeconds: 20
          timeoutSeconds: 3
          failureThreshold: 3
        resources:
          requests: {{cpu: {requests[0]}, memory: {requests[1]}}}
          limits: {{cpu: {limits[0]}, memory: {limits[1]}}}
"""


def service(name, port):
    return f"""---
apiVersion: v1
kind: Service
metadata:
  name: {name}
  namespace: {NS}
  labels: {{app: {name}}}
spec:
  selector: {{app: {name}}}
  ports:
  - port: {port}
    targetPort: {port}
"""


out = ["# GENERATED by gen-manifests.py — do not hand-edit. Regenerate with:",
       f"#   ./gen-manifests.py {TAG}", ""]

for name, port, db, extra in GO_SERVICES:
    env = {"PORT": str(port), "SERVICE_NAME": name}
    if db:
        env["DB_NAME"] = db
    env.update(extra)
    out.append(deployment(name, port, image_ref(f"nemo-{name}"), env,
                          "/actuator/health", ("100m", "128Mi"), ("500m", "512Mi")))
    out.append(service(name, port))

for name, port, image, health, extra in ML_SERVICES:
    out.append(deployment(name, port, image, {"PORT": str(port), **extra}, health,
                          ("50m", "256Mi"), ("1", "1Gi"),
                          envfrom_secret=False, initial_delay=20))
    out.append(service(name, port))

# Portal: keep the historical name (ingress + NodePort service point at it).
out.append(f"""---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: lms-portal-ui
  namespace: {NS}
  labels: {{app: lms-portal-ui}}
spec:
  replicas: 1
  selector:
    matchLabels: {{app: lms-portal-ui}}
  template:
    metadata:
      labels: {{app: lms-portal-ui}}
    spec:
{PULL_SECRETS}      containers:
      - name: lms-portal-ui
        image: {image_ref("nemo-portal")}
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 3000
        readinessProbe:
          httpGet: {{path: /, port: 3000}}
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests: {{cpu: 25m, memory: 32Mi}}
          limits: {{cpu: 250m, memory: 128Mi}}
""")

print("\n".join(out))
