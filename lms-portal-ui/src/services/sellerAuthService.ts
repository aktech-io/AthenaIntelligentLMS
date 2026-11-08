/**
 * Seller (SME) authentication via bff-gateway phone + OTP.
 *
 * nginx: /proxy/bff/ → bff-gateway:8110 (see nginx.conf). Go shapes from
 * go-services/internal/bff/gateway/service/auth_service.go:
 *   POST /api/v1/mobile/auth/otp/send   {phoneNumber, purpose?, tenantId?}
 *     → {message, expiresInSeconds, otp?}        (otp only when OTP_ECHO_ENABLED=true)
 *   POST /api/v1/mobile/auth/otp/verify {phoneNumber, otp, purpose?, tenantId?}
 *     → {accessToken, refreshToken, user:{id, phoneNumber, fullName?, status, kycTier, hasPin}}
 *
 * The access token is stored under `athena_jwt` (same key as the portal) so
 * invoiceService works unchanged.
 */

const BFF = "/proxy/bff/api/v1/mobile/auth";
const REFRESH_KEY = "athena_refresh";
const PHONE_KEY = "nemo_seller_phone";

export interface SendOtpResponse {
  message?: string;
  expiresInSeconds?: number;
  /** echoed only in dev/sandbox (OTP_ECHO_ENABLED) */
  otp?: string;
}

export interface MobileUser {
  id: string;
  phoneNumber: string;
  fullName?: string | null;
  status?: string;
  kycTier?: number;
  hasPin?: boolean;
  /** account-service customer business id, when the BFF includes it */
  customerId?: string;
  businessName?: string;
}

export interface VerifyOtpResponse {
  accessToken: string;
  refreshToken?: string;
  user: MobileUser;
}

export class SellerAuthError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "SellerAuthError";
    this.status = status;
  }
}

async function post<T>(url: string, body: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  } catch (e) {
    throw new SellerAuthError(e instanceof Error ? e.message : "Network error", 0);
  }
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    data = null;
  }
  if (!res.ok) {
    const msg =
      typeof data === "object" && data !== null && "message" in data
        ? String((data as Record<string, unknown>).message)
        : res.status === 401 || res.status === 400
          ? "That code didn't match — check the OTP and try again."
          : res.status === 429
            ? "Too many attempts. Wait a minute and try again."
            : res.status === 502 || res.status === 503 || res.status === 504
              ? "Sign-in service unreachable. Try again shortly."
              : `Request failed with status ${res.status}`;
    throw new SellerAuthError(msg, res.status);
  }
  return data as T;
}

/** Normalise a Kenyan/any phone into E.164-ish: strips spaces, converts leading 0 → +254 when no country code. */
export function normalisePhone(input: string): string {
  const raw = input.replace(/[\s()-]/g, "");
  if (!raw) return "";
  if (raw.startsWith("+")) return raw;
  if (raw.startsWith("00")) return `+${raw.slice(2)}`;
  if (/^0\d{9}$/.test(raw)) return `+254${raw.slice(1)}`;
  if (/^254\d{9}$/.test(raw)) return `+${raw}`;
  if (/^\d{9}$/.test(raw)) return `+254${raw}`;
  return raw;
}

export const sellerAuthService = {
  async sendOtp(phone: string, tenantId?: string): Promise<SendOtpResponse> {
    const phoneNumber = normalisePhone(phone);
    // `phoneNumber` is the field the Go handler decodes; `phone` is sent too for forward-compat.
    return post<SendOtpResponse>(`${BFF}/otp/send`, { phoneNumber, phone: phoneNumber, purpose: "LOGIN", tenantId: tenantId || undefined });
  },

  async verifyOtp(phone: string, otp: string, tenantId?: string): Promise<VerifyOtpResponse> {
    const phoneNumber = normalisePhone(phone);
    const raw = await post<Record<string, unknown>>(`${BFF}/otp/verify`, { phoneNumber, phone: phoneNumber, otp: otp.trim(), purpose: "LOGIN", tenantId: tenantId || undefined });
    const token = (raw.accessToken ?? raw.token ?? raw.jwt) as string | undefined;
    if (!token) throw new SellerAuthError("Sign-in succeeded but no token was returned.", 500);
    const user = ((raw.user as MobileUser | undefined) ?? { id: "", phoneNumber }) as MobileUser;
    if (!user.phoneNumber) user.phoneNumber = phoneNumber;
    const refresh = raw.refreshToken as string | undefined;
    try {
      if (refresh) localStorage.setItem(REFRESH_KEY, refresh);
      localStorage.setItem(PHONE_KEY, phoneNumber);
    } catch {
      /* storage unavailable */
    }
    return { accessToken: token, refreshToken: refresh, user };
  },

  lastPhone(): string {
    try {
      return localStorage.getItem(PHONE_KEY) ?? "";
    } catch {
      return "";
    }
  },

  clear(): void {
    try {
      localStorage.removeItem(REFRESH_KEY);
    } catch {
      /* ignore */
    }
  },
};

/** Decode a JWT payload without verifying (UI hints only: customerId, name, phone). */
export function decodeJwt(token: string | null): Record<string, unknown> | null {
  if (!token) return null;
  try {
    const part = token.split(".")[1];
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4))) as Record<string, unknown>;
  } catch {
    return null;
  }
}
