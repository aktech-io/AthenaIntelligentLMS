// Runtime configuration — rewritten by docker-entrypoint.sh at container start.
// The __PLACEHOLDER__ tokens are replaced from env (APP_MODE, DEMO_MODE,
// PUBLIC_VERIFY_BASE). When served unmodified (npm run dev / vite preview),
// src/lib/appConfig.ts ignores the placeholders and falls back to VITE_* vars.
window.__NEMO_CONFIG__ = {
  mode: "__APP_MODE__",
  demo: "__DEMO_MODE__",
  publicVerifyBase: "__PUBLIC_VERIFY_BASE__",
  tenantId: "__TENANT_ID__",
};
