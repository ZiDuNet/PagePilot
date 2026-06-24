export class APIError extends Error {
  status: number;
  body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.body = body;
  }
}

export function authHeaders(extra: HeadersInit = {}): HeadersInit {
  const headers: Record<string, string> = { ...(extra as Record<string, string>) };
  const token =
    localStorage.getItem("hostctl-admin-token") ||
    localStorage.getItem("hostctl-token") ||
    "";
  if (token && !headers.Authorization) headers.Authorization = `Bearer ${token}`;
  return headers;
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = authHeaders(init.headers || {});
  const res = await fetch(path, {
    credentials: "same-origin",
    cache: init.method ? "no-store" : "default",
    ...init,
    headers
  });
  const text = await res.text();
  let body: unknown = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!res.ok) {
    const data = body as { detail?: string; errorCode?: string };
    throw new APIError(data?.detail || data?.errorCode || `HTTP ${res.status}`, res.status, body);
  }
  return body as T;
}

export function publicURL(path: string): string {
  if (/^https?:\/\//i.test(path)) return path;
  return `${location.origin}${path}`;
}
