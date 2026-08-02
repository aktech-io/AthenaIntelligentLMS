/**
 * Client-side RBAC hints from the stored JWT — used ONLY for UI affordances
 * (hiding nav entries the caller cannot use). Enforcement is always
 * server-side (auth.RequirePermission in go-services); this mirrors its
 * precedence so the UI agrees with the backend:
 *
 *  1. token carries a `permissions` claim → check membership;
 *  2. no permissions claim (legacy token)  → fall back to a role check.
 */

const JWT_KEY = "athena_jwt"; // same key as src/lib/api.ts

interface JwtPayload {
  permissions?: unknown;
  roles?: unknown;
}

function decodeJwtPayload(token: string): JwtPayload | null {
  const part = token.split(".")[1];
  if (!part) return null;
  const b64 = part.replace(/-/g, "+").replace(/_/g, "/");
  const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
  return JSON.parse(atob(padded)) as JwtPayload;
}

export function hasPermission(permission: string, fallbackRoles: string[] = []): boolean {
  try {
    const token = localStorage.getItem(JWT_KEY);
    if (!token) return false;
    const payload = decodeJwtPayload(token);
    if (!payload) return false;
    if (Array.isArray(payload.permissions)) {
      return payload.permissions.includes(permission);
    }
    const roles = Array.isArray(payload.roles)
      ? payload.roles.map((r) => String(r).toUpperCase())
      : [];
    return fallbackRoles.some((r) => roles.includes(r.toUpperCase()));
  } catch {
    return false;
  }
}
