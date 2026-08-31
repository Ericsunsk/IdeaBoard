import type { AuthGuardSettings } from "./authGuardSettings.js";
import {
  isRateLimitedAuthPath,
  isSignUpPath,
  normalizeAuthPath,
} from "../infrastructure/authUtils.js";
import { normalizeOrigin } from "../infrastructure/origin.js";
import { supportedSocialProviders } from "../social-identity/catalog.js";

type RateLimitEntry = {
  count: number;
  resetAtMs: number;
};

export type AuthRegistrationEntry =
  | "email_otp"
  | "social_oauth"
  | `${string}_oauth`;

export type AuthAdmissionDecision =
  | {
      allowed: true;
      corsOrigin?: string;
      preflight?: boolean;
    }
  | {
      allowed: false;
      reason:
        | "origin_not_allowed"
        | "registration_disabled"
        | "mail_not_ready"
        | "mail_unavailable"
        | "provider_not_ready"
        | "rate_limited"
        | "admission_unavailable";
      corsOrigin?: string;
      retryAfterSeconds?: number;
    };

export type AuthAdmissionRequest = {
  method: string;
  path: string;
  origin: string;
  requestIP: string;
  fallbackIP?: string;
};

export type AuthAdmissionPolicy = {
  admitRequest(input: AuthAdmissionRequest): Promise<AuthAdmissionDecision>;
  admitRegistration(
    entry: AuthRegistrationEntry,
  ): Promise<AuthAdmissionDecision>;
};

export type AuthAdmissionPolicyOptions = {
  trustedOrigins: string[];
  getSettings: () => Promise<AuthGuardSettings>;
  isMailReady?: () => Promise<boolean>;
  getSocialProviderReadiness?: (provider: string) => Promise<boolean>;
  onDependencyError?: (
    dependency: "settings" | "mail" | "provider",
    error: unknown,
  ) => void;
};

export function createAuthAdmissionPolicy(
  options: AuthAdmissionPolicyOptions,
): AuthAdmissionPolicy {
  const allowedOrigins = new Set(
    options.trustedOrigins.map(normalizeOrigin).filter(Boolean),
  );
  const rateLimits = new Map<string, RateLimitEntry>();
  let rateLimitChecks = 0;

  return {
    async admitRequest(input) {
      const rawOrigin = input.origin.trim();
      const corsOrigin = rawOrigin ? normalizeOrigin(rawOrigin) : "";
      if (rawOrigin && (!corsOrigin || !allowedOrigins.has(corsOrigin))) {
        return { allowed: false, reason: "origin_not_allowed" };
      }

      if (corsOrigin && input.method.toUpperCase() === "OPTIONS") {
        return {
          allowed: true,
          ...(corsOrigin ? { corsOrigin } : {}),
          preflight: true,
        };
      }

      const settings = await readSettings(options);
      if (!settings) {
        return {
          allowed: false,
          reason: "admission_unavailable",
          ...(corsOrigin ? { corsOrigin } : {}),
        };
      }
      const path = normalizeAuthPath(input.path);
      if (isSignUpPath(path)) {
        const denied = await checkRegistrationDependencies(
          options,
          "email_credential",
          settings,
        );
        if (denied) {
          return {
            ...denied,
            ...(corsOrigin ? { corsOrigin } : {}),
          };
        }
      }

      if (!isSignUpPath(path) && requiresMailReadiness(path)) {
        const denied = await checkMailReadiness(options);
        if (denied) {
          return {
            ...denied,
            ...(corsOrigin ? { corsOrigin } : {}),
          };
        }
      }

      if (isAdmissionRateLimitedPath(path)) {
        const rateLimit = checkRateLimit(
          rateLimits,
          buildRateLimitKey(path, input.requestIP, input.fallbackIP),
          settings.windowSeconds,
          settings.maxRequests,
        );
        if (!rateLimit.allowed) {
          return {
            allowed: false,
            reason: "rate_limited",
            retryAfterSeconds: rateLimit.retryAfterSeconds,
            ...(corsOrigin ? { corsOrigin } : {}),
          };
        }
        rateLimitChecks += 1;
        if (rateLimitChecks % 200 === 0) {
          cleanupExpiredRateLimits(rateLimits, Date.now());
        }
      }

      return {
        allowed: true,
        ...(corsOrigin ? { corsOrigin } : {}),
      };
    },

    async admitRegistration(entry) {
      const settings = await readSettings(options);
      if (!settings) {
        return { allowed: false, reason: "admission_unavailable" };
      }
      return (
        (await checkRegistrationDependencies(options, entry, settings)) ?? {
          allowed: true,
        }
      );
    },
  };
}

type RegistrationDependencyEntry =
  | AuthRegistrationEntry
  | "email_credential";

async function checkRegistrationDependencies(
  options: AuthAdmissionPolicyOptions,
  entry: RegistrationDependencyEntry,
  settings: AuthGuardSettings,
): Promise<AuthAdmissionDecision | null> {
  if (!settings.registrationEnabled) {
    return { allowed: false, reason: "registration_disabled" };
  }
  if (entry === "email_credential" || entry === "email_otp") {
    return checkMailReadiness(options);
  }
  const provider = entry.endsWith("_oauth") && entry !== "social_oauth"
    ? entry.slice(0, -"_oauth".length)
    : undefined;
  if (!options.getSocialProviderReadiness) {
    return { allowed: false, reason: "provider_not_ready" };
  }
  try {
    const ready = provider
      ? await options.getSocialProviderReadiness(provider)
      : (
          await Promise.all(
            supportedSocialProviders.map((candidate) =>
              options.getSocialProviderReadiness!(candidate),
            ),
          )
        ).some(Boolean);
    return ready
      ? null
      : { allowed: false, reason: "provider_not_ready" };
  } catch (error) {
    reportDependencyError(options, "provider", error);
    return { allowed: false, reason: "provider_not_ready" };
  }
}

async function checkMailReadiness(
  options: AuthAdmissionPolicyOptions,
): Promise<AuthAdmissionDecision | null> {
  if (!options.isMailReady) return null;
  try {
    return (await options.isMailReady())
      ? null
      : { allowed: false, reason: "mail_not_ready" };
  } catch (error) {
    reportDependencyError(options, "mail", error);
    return { allowed: false, reason: "mail_unavailable" };
  }
}

async function readSettings(
  options: AuthAdmissionPolicyOptions,
): Promise<AuthGuardSettings | null> {
  try {
    return await options.getSettings();
  } catch (error) {
    reportDependencyError(options, "settings", error);
    return null;
  }
}

function reportDependencyError(
  options: AuthAdmissionPolicyOptions,
  dependency: "settings" | "mail" | "provider",
  error: unknown,
): void {
  if (options.onDependencyError) {
    options.onDependencyError(dependency, error);
    return;
  }
  console.error(`[auth] Admission ${dependency} dependency failed`, error);
}

function requiresMailReadiness(path: string): boolean {
  return (
    isSignUpPath(path) ||
    path === "/email-otp/send-verification-otp" ||
    path === "/request-password-reset" ||
    path === "/email-otp/request-password-reset" ||
    path === "/forget-password/email-otp" ||
    path === "/email-otp/reset-password" ||
    path === "/email-otp/request-email-change" ||
    path === "/email-otp/change-email"
  );
}

function isAdmissionRateLimitedPath(path: string): boolean {
  return (
    isRateLimitedAuthPath(path) ||
    path === "/email-otp/send-verification-otp"
  );
}

function buildRateLimitKey(
  path: string,
  requestIP: string,
  fallbackIP = "",
): string {
  const bucket = isSignUpPath(path) ? "signup" : "signin";
  const ip = requestIP.trim() || fallbackIP.trim() || "unknown";
  return `${bucket}:${ip}`;
}

function checkRateLimit(
  store: Map<string, RateLimitEntry>,
  key: string,
  windowSeconds: number,
  maxRequests: number,
): { allowed: boolean; retryAfterSeconds: number } {
  const nowMs = Date.now();
  let entry = store.get(key);
  if (!entry || nowMs >= entry.resetAtMs) {
    entry = { count: 0, resetAtMs: nowMs + windowSeconds * 1_000 };
  }
  if (entry.count >= maxRequests) {
    store.set(key, entry);
    return {
      allowed: false,
      retryAfterSeconds: Math.max(
        1,
        Math.ceil((entry.resetAtMs - nowMs) / 1_000),
      ),
    };
  }
  entry.count += 1;
  store.set(key, entry);
  return { allowed: true, retryAfterSeconds: 0 };
}

function cleanupExpiredRateLimits(
  store: Map<string, RateLimitEntry>,
  nowMs: number,
): void {
  for (const [key, entry] of store) {
    if (entry.resetAtMs <= nowMs) store.delete(key);
  }
}
