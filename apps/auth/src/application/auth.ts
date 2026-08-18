import { betterAuth, type BetterAuthOptions } from "better-auth";
import { createAuthMiddleware } from "better-auth/api";
import { admin, emailOTP } from "better-auth/plugins";
import { Pool, type PoolConfig } from "pg";
import type { AuthAdmissionPolicy } from "../admission/authAdmissionPolicy.js";
import { createAuthAdmissionPolicy } from "../admission/authAdmissionPolicy.js";
import { createAuthGuardSettingsReader } from "../admission/authGuardSettings.js";
import { normalizeCanonicalEmail } from "../infrastructure/authUtils.js";
import type { AuthEnvironment } from "../infrastructure/env.js";
import {
  passwordResetOtp,
  verificationOtp,
} from "../transactional-mail/emailTemplates/index.js";
import {
  applySocialOAuthRequestPolicy,
  createRegistrationUserCreationGuard,
} from "../admission/socialOAuthPolicy.js";
import { createRuntimeAuthRegistry } from "../social-identity/runtimeAuthRegistry.js";
import type { MailConfigService } from "../transactional-mail/mailConfig.js";
import { createRuntimeMailConfigService } from "../transactional-mail/mailConfigAdapters.js";
import {
  createRuntimeSocialProviderSettingsService,
} from "../social-identity/socialProviderSettingsAdapters.js";
import {
  type SocialProviderSettingsService,
} from "../social-identity/socialProviderSettings.js";

const EMAIL_OTP_EXPIRES_IN_SECONDS = 600;
const EMAIL_OTP_EXPIRES_IN_MINUTES = EMAIL_OTP_EXPIRES_IN_SECONDS / 60;
const CANONICAL_EMAIL_PATHS = new Set([
  "/sign-in/email",
  "/sign-up/email",
  "/request-password-reset",
  "/reset-password",
  "/change-email",
  "/email-otp/request-password-reset",
  "/email-otp/reset-password",
  "/email-otp/request-email-change",
  "/email-otp/change-email",
  "/email-otp/send-verification-otp",
  "/email-otp/verify-email",
]);

export type AuthDatabase = Pool;

export type AuthComponents = {
  mailConfigService: MailConfigService;
  authAdmissionPolicy: AuthAdmissionPolicy;
  runtimeAuthRegistry: ReturnType<typeof createRuntimeAuthRegistry>;
  socialProviderSettingsService: SocialProviderSettingsService;
};

export function createAuthPool(config: AuthEnvironment["database"]): Pool {
  const poolConfig: PoolConfig = {
    host: config.host,
    port: config.port,
    user: config.user,
    password: config.password,
    database: config.database,
    ssl:
      config.sslMode === "disable"
        ? false
        : { rejectUnauthorized: config.sslMode === "verify-full" },
    max: 10,
    connectionTimeoutMillis: 5_000,
    idleTimeoutMillis: 30_000,
  };
  return new Pool(poolConfig);
}

export function createAuthComponents(
  environment: AuthEnvironment,
  database: AuthDatabase,
): AuthComponents {
  const mailConfigService = createRuntimeMailConfigService(
    database,
    environment.secrets.encryptionKey,
  );
  const getAuthGuardSettings = createAuthGuardSettingsReader(database);
  let socialProviderSettingsService: SocialProviderSettingsService | undefined;
  const authAdmissionPolicy = createAuthAdmissionPolicy({
    trustedOrigins: environment.auth.trustedOrigins,
    getSettings: getAuthGuardSettings,
    isMailReady: async () =>
      (await mailConfigService.getSummary()).readiness === "ready",
    getSocialProviderReadiness: async (provider) =>
      Boolean(
        socialProviderSettingsService &&
          (await socialProviderSettingsService.getCapabilities()).social[provider],
      ),
  });
  const authBaseOptions = createBetterAuthOptions({
    environment,
    database,
    mailConfigService,
    authAdmissionPolicy,
  });
  const runtimeAuthRegistry = createRuntimeAuthRegistry(betterAuth, {
    baseOptions: authBaseOptions,
  });
  socialProviderSettingsService = createRuntimeSocialProviderSettingsService(
    database,
    environment.auth.baseURL,
    environment.secrets.encryptionKey,
    (config) => runtimeAuthRegistry.apply(config),
  );

  return {
    mailConfigService,
    authAdmissionPolicy,
    runtimeAuthRegistry,
    socialProviderSettingsService,
  };
}

export function createBetterAuthOptions(input: {
  environment: AuthEnvironment;
  database: BetterAuthOptions["database"];
  mailConfigService: Pick<MailConfigService, "send">;
  authAdmissionPolicy: AuthAdmissionPolicy;
}): BetterAuthOptions {
  const { environment, database, mailConfigService, authAdmissionPolicy } = input;
  return {
    database,
    basePath: "/api/auth",
    appName: "IdeaBoard",
    ...(environment.auth.secret ? { secret: environment.auth.secret } : {}),
    ...(environment.auth.baseURL ? { baseURL: environment.auth.baseURL } : {}),
    trustedOrigins: environment.auth.trustedOrigins,
    emailAndPassword: {
      enabled: true,
      autoSignIn: false,
      requireEmailVerification: true,
      minPasswordLength: 10,
      maxPasswordLength: 128,
      revokeSessionsOnPasswordReset: true,
    },
    emailVerification: {
      autoSignInAfterVerification: true,
    },
    session: {
      expiresIn: 60 * 60 * 24,
      updateAge: 60 * 60,
    },
    plugins: [
      emailOTP({
        otpLength: 6,
        expiresIn: EMAIL_OTP_EXPIRES_IN_SECONDS,
        allowedAttempts: 5,
        storeOTP: "hashed",
        resendStrategy: "rotate",
        sendVerificationOnSignUp: true,
        overrideDefaultEmailVerification: true,
        changeEmail: { enabled: true, verifyCurrentEmail: false },
        rateLimit: { window: 60 * 60, max: 5 },
        sendVerificationOTP: async ({ email, otp, type }) => {
          const content =
            type === "forget-password"
              ? passwordResetOtp({
                  otp,
                  expiresInMinutes: EMAIL_OTP_EXPIRES_IN_MINUTES,
                })
              : verificationOtp({
                  otp,
                  expiresInMinutes: EMAIL_OTP_EXPIRES_IN_MINUTES,
                });
          await mailConfigService.send({ to: email, content });
        },
      }),
      admin({
        defaultRole: "user",
        adminRoles: ["admin"],
        impersonationSessionDuration: 60 * 60,
      }),
    ],
    databaseHooks: {
      user: {
        create: {
          before: createRegistrationUserCreationGuard(authAdmissionPolicy),
        },
      },
    },
    hooks: {
      before: createAuthMiddleware(async (ctx) => {
        await applySocialOAuthRequestPolicy(ctx, authAdmissionPolicy);
        if (CANONICAL_EMAIL_PATHS.has(ctx.path)) {
          if (typeof ctx.body?.email === "string") {
            ctx.body.email = normalizeCanonicalEmail(ctx.body.email);
          }
          if (typeof ctx.body?.newEmail === "string") {
            ctx.body.newEmail = normalizeCanonicalEmail(ctx.body.newEmail);
          }
        }
      }),
    },
  } satisfies BetterAuthOptions;
}
