import { APIError } from "better-auth/api";
import type {
  AuthAdmissionDecision,
  AuthAdmissionPolicy,
  AuthRegistrationEntry,
} from "./authAdmissionPolicy.js";

type AuthHookContext = {
  path: string;
  body?: unknown;
};

type UserCreationContext = {
  path?: unknown;
} | null;

export async function applyGoogleOAuthRequestPolicy(
  context: AuthHookContext,
  admissionPolicy: AuthAdmissionPolicy,
): Promise<void> {
  if (!context.body || typeof context.body !== "object") return;
  const body = context.body as Record<string, unknown>;
  if (body.provider !== "google") return;
  if (context.path !== "/sign-in/social" && context.path !== "/link-social") {
    return;
  }

  // Better Auth appends request scopes to the provider defaults. Removing the
  // caller value keeps the server-side provider definition as the allowlist.
  delete body.scopes;
  if (context.path === "/sign-in/social") {
    const decision = await admissionPolicy.admitRegistration("google_oauth");
    body.requestSignUp = decision.allowed;
  }
}

export function createRegistrationUserCreationGuard(
  admissionPolicy: AuthAdmissionPolicy,
) {
  return async (
    user: { emailVerified?: unknown },
    context?: UserCreationContext,
  ): Promise<void> => {
    // Credential users are created unverified and already pass the HTTP
    // admission decision. Verified creation is either Email OTP or OAuth.
    if (user.emailVerified !== true) return;
    const decision = await admissionPolicy.admitRegistration(
      registrationEntryFromContext(context),
    );
    if (!decision.allowed) throw admissionAPIError(decision);
  };
}

function admissionAPIError(
  decision: Extract<AuthAdmissionDecision, { allowed: false }>,
): APIError {
  if (decision.reason === "registration_disabled") {
    return new APIError("FORBIDDEN", {
      code: "REGISTRATION_DISABLED",
      message: "注册已关闭",
    });
  }
  if (
    decision.reason === "mail_not_ready" ||
    decision.reason === "mail_unavailable"
  ) {
    return new APIError("SERVICE_UNAVAILABLE", {
      code: "MAIL_UNAVAILABLE",
      message: "邮件服务暂不可用",
    });
  }
  return new APIError("SERVICE_UNAVAILABLE", {
    code: "AUTH_ADMISSION_UNAVAILABLE",
    message: "认证服务暂不可用",
  });
}

function registrationEntryFromContext(
  context?: UserCreationContext,
): AuthRegistrationEntry {
  return context?.path === "/sign-in/email-otp"
    ? "email_otp"
    : "google_oauth";
}
