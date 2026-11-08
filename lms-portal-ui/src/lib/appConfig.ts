/**
 * Runtime app configuration.
 *
 * Production: nginx serves /config.js (written by docker-entrypoint.sh from
 * APP_MODE / DEMO_MODE / PUBLIC_VERIFY_BASE) which sets window.__NEMO_CONFIG__
 * before the bundle loads. Dev: public/config.js still ships the unreplaced
 * __PLACEHOLDER__ tokens, so we fall back to VITE_APP_MODE / VITE_DEMO_MODE /
 * VITE_PUBLIC_VERIFY_BASE (e.g. `VITE_APP_MODE=invoice-client npm run dev`).
 */

export type AppMode = "portal" | "invoice-admin" | "invoice-client";

export interface AppConfig {
  mode: AppMode;
  demo: boolean;
  /** Origin for public verify links/QR; empty = current origin. */
  publicVerifyBase: string;
  /** Tenant the seller app signs mobile users into (same tenant as the bank's staff). */
  tenantId: string;
}

declare global {
  interface Window {
    __NEMO_CONFIG__?: { mode?: unknown; demo?: unknown; publicVerifyBase?: unknown; tenantId?: unknown };
  }
}

const MODES: AppMode[] = ["portal", "invoice-admin", "invoice-client"];

const isPlaceholder = (v: unknown) => typeof v === "string" && /^__[A-Z_]+__$/.test(v);

function pick(runtime: unknown, dev: string | undefined): string | undefined {
  if (runtime !== undefined && runtime !== null && !isPlaceholder(runtime)) return String(runtime);
  return dev;
}

function parseBool(v: string | undefined): boolean {
  return /^(1|true|yes|on)$/i.test((v ?? "").trim());
}

function load(): AppConfig {
  const rt = (typeof window !== "undefined" && window.__NEMO_CONFIG__) || {};
  const env = (import.meta as unknown as { env?: Record<string, string | undefined> }).env ?? {};
  const modeRaw = (pick(rt.mode, env.VITE_APP_MODE) ?? "portal").trim().toLowerCase();
  const mode = (MODES as string[]).includes(modeRaw) ? (modeRaw as AppMode) : "portal";
  const demoRaw = pick(rt.demo, env.VITE_DEMO_MODE);
  const demo = typeof rt.demo === "boolean" ? rt.demo : parseBool(demoRaw);
  const publicVerifyBase = (pick(rt.publicVerifyBase, env.VITE_PUBLIC_VERIFY_BASE) ?? "").replace(/\/+$/, "");
  const tenantId = (pick(rt.tenantId, env.VITE_TENANT_ID) ?? "default").trim() || "default";
  return { mode, demo, publicVerifyBase, tenantId };
}

export const appConfig: AppConfig = load();

export const isPortalMode = () => appConfig.mode === "portal";
export const isInvoiceAdminMode = () => appConfig.mode === "invoice-admin";
export const isInvoiceClientMode = () => appConfig.mode === "invoice-client";

/** Brand strings per mode (sidebar header, document title). */
export const branding = (() => {
  switch (appConfig.mode) {
    case "invoice-admin":
      return { title: "Nemo Invoice Finance", brand: "Nemo Invoice Finance", tagline: "Invoice Finance · Admin", home: "/invoice-finance" };
    case "invoice-client":
      return { title: "Nemo Invoice Finance", brand: "Nemo Invoice Finance", tagline: "Seller app", home: "/" };
    default:
      return { title: "Nemo — Neobank in a Box", brand: "Nemo", tagline: "Neobank in a Box", home: "/" };
  }
})();

/** Apply mode branding to the document (title, theme-color is shared). */
export function applyBranding(): void {
  if (typeof document === "undefined") return;
  document.title = branding.title;
  document.documentElement.setAttribute("data-app-mode", appConfig.mode);
}

/** Public verify URL for a fingerprint, honouring PUBLIC_VERIFY_BASE. */
export function publicVerifyUrl(fingerprint: string): string {
  const origin = appConfig.publicVerifyBase || (typeof window !== "undefined" ? window.location.origin : "");
  return `${origin}/verify/${encodeURIComponent(fingerprint)}`;
}
