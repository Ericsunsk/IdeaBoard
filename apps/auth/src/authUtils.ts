export type InternalUserShape = {
  id: string;
  username?: string | null;
  email?: string | null;
  emailVerified?: boolean | null;
  name?: string | null;
  role?: string | null;
};

export function getAuthSubpath(rawURL: string): string {
  const path = normalizeURLPath(rawURL);
  if (path === "/api/auth") return "/";
  if (path.startsWith("/api/auth/")) {
    return path.slice("/api/auth".length);
  }
  return path;
}

export function normalizeURLPath(rawURL: string): string {
  const rawPath = String(rawURL || "").split("?")[0] || "/";
  return normalizeAuthPath(rawPath);
}

export function normalizeAuthPath(rawPath: string): string {
  const path = String(rawPath || "").trim();
  if (!path) return "/";
  if (path.startsWith("/")) return path;
  return `/${path}`;
}

export function isSignUpPath(path: string): boolean {
  return (
    path === "/sign-up" ||
    path === "/sign-up/email" ||
    path.startsWith("/sign-up/")
  );
}

export function isRateLimitedAuthPath(path: string): boolean {
  return (
    path === "/sign-in" ||
    path === "/sign-up" ||
    path.startsWith("/sign-in/") ||
    path.startsWith("/sign-up/")
  );
}

export function clampInteger(
  value: unknown,
  min: number,
  max: number,
  fallback: number,
): number {
  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  if (parsed < min || parsed > max) {
    return fallback;
  }
  return parsed;
}

export function normalizeUsername(raw: unknown): string {
  const text = String(raw || "")
    .trim()
    .replace(/[^a-zA-Z0-9_-]/g, "_")
    .replace(/_+/g, "_")
    .replace(/^_+|_+$/g, "");
  return text.slice(0, 50);
}

export function normalizeRegistrationUsername(raw: unknown): string {
  return normalizeUsername(raw).toLowerCase();
}

export function normalizeCanonicalEmail(raw: unknown): string {
  return String(raw || "")
    .trim()
    .toLowerCase();
}

export function isValidCanonicalEmail(raw: unknown): boolean {
  const email = normalizeCanonicalEmail(raw);
  const at = email.lastIndexOf("@");
  if (
    !email ||
    email.length > 320 ||
    at <= 0 ||
    at === email.length - 1 ||
    email.includes(" ")
  ) {
    return false;
  }
  const domain = email.slice(at + 1);
  return (
    email.slice(0, at).length <= 64 &&
    domain.includes(".") &&
    !domain.startsWith(".") &&
    !domain.endsWith(".")
  );
}

export function isReservedRegistrationUsername(raw: unknown): boolean {
  const username = normalizeRegistrationUsername(raw);
  return username === "admin" || username.startsWith("deleted-user-");
}

export function normalizeAuthRole(raw: unknown): "admin" | "user" | null {
  const value = String(raw ?? "user")
    .trim()
    .toLowerCase();
  if (value === "admin" || value === "user") {
    return value;
  }
  return null;
}

export function normalizeOptionalText(
  raw: unknown,
  maxLength: number,
): string | null {
  const text = String(raw ?? "").trim();
  if (!text) return null;
  return text.slice(0, maxLength);
}

export function normalizeOptionalPositiveInteger(raw: unknown): number | null {
  if (raw == null || raw === "") return null;
  const value = Number.parseInt(String(raw), 10);
  if (!Number.isFinite(value) || value <= 0) {
    return null;
  }
  return value;
}

export function extractAuthUser(payload: unknown): InternalUserShape | null {
  const candidate = payload as {
    user?: unknown;
    data?: { user?: unknown };
  };
  const user = (candidate?.user || candidate?.data?.user || candidate) as
    InternalUserShape | undefined;
  if (!user || typeof user.id !== "string" || !user.id.trim()) {
    return null;
  }
  return user;
}

export function extractErrorMessage(error: unknown, fallback: string): string {
  const err = error as {
    message?: unknown;
    error?: { message?: unknown };
    data?: { message?: unknown; msg?: unknown };
  };
  if (typeof err?.message === "string" && err.message.trim())
    return err.message;
  if (typeof err?.error?.message === "string" && err.error.message.trim())
    return err.error.message;
  if (typeof err?.data?.message === "string" && err.data.message.trim())
    return err.data.message;
  if (typeof err?.data?.msg === "string" && err.data.msg.trim())
    return err.data.msg;
  return fallback;
}

export function isDuplicateUserError(error: unknown): boolean {
  const message = extractErrorMessage(error, "").toLowerCase();
  return (
    message.includes("already exists") ||
    message.includes("already taken") ||
    message.includes("duplicate") ||
    message.includes("unique")
  );
}
