/**
 * AthenaLMS API Client
 * Fetch-based HTTP client with JWT auth, error handling, and typed responses.
 */

const JWT_KEY = "athena_jwt";

export interface ApiResult<T> {
  data: T | null;
  error: string | null;
  status: number;
}

function getToken(): string | null {
  try {
    return localStorage.getItem(JWT_KEY);
  } catch {
    return null;
  }
}

function buildHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...extra,
  };
  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  return headers;
}

async function request<T>(
  url: string,
  options: RequestInit = {}
): Promise<ApiResult<T>> {
  try {
    const response = await fetch(url, {
      ...options,
      headers: buildHeaders((options.headers as Record<string, string>) ?? {}),
    });

    const status = response.status;

    if (status === 204) {
      return { data: null, error: null, status };
    }

    let body: unknown;
    try {
      body = await response.json();
    } catch {
      body = null;
    }

    if (!response.ok) {
      const errorMsg =
        typeof body === "object" && body !== null && "message" in body
          ? String((body as Record<string, unknown>).message)
          : `Request failed with status ${status}`;
      return { data: null, error: errorMsg, status };
    }

    return { data: body as T, error: null, status };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Network error";
    return { data: null, error: message, status: 0 };
  }
}

export async function apiGet<T>(url: string): Promise<ApiResult<T>> {
  return request<T>(url, { method: "GET" });
}

/**
 * Fetches a file (e.g. CSV export) with JWT auth and triggers a browser
 * download. The response is read as a blob and saved via a temporary anchor,
 * which keeps the Authorization header attached (unlike a plain link click).
 */
export async function downloadFile(url: string, filename: string): Promise<void> {
  const response = await fetch(url, {
    method: "GET",
    headers: buildHeaders(),
  });
  if (!response.ok) {
    throw new Error(`Download failed with status ${response.status}`);
  }
  const blob = await response.blob();
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(objectUrl);
}

export async function apiPost<T>(url: string, body?: unknown): Promise<ApiResult<T>> {
  return request<T>(url, {
    method: "POST",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function apiPut<T>(url: string, body?: unknown): Promise<ApiResult<T>> {
  return request<T>(url, {
    method: "PUT",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function apiDelete<T>(url: string): Promise<ApiResult<T>> {
  return request<T>(url, { method: "DELETE" });
}

export async function apiPatch<T>(url: string, body?: unknown): Promise<ApiResult<T>> {
  return request<T>(url, {
    method: "PATCH",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

/**
 * Page response shape used by Spring Data pageable endpoints.
 */
export interface PageResponse<T> {
  content: T[];
  totalElements: number;
  totalPages: number;
  size: number;
  number: number;
  first: boolean;
  last: boolean;
}
