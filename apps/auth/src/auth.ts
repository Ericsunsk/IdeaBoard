import { betterAuth, type BetterAuthOptions } from "better-auth";
import { createAuthMiddleware } from "better-auth/api";
import { admin, emailOTP, haveIBeenPwned } from "better-auth/plugins";
import { Pool } from "pg";
import { env } from "./env.js";
import { normalizeCanonicalEmail } from "./authUtils.js";
import {
  applyGoogleOAuthRequestPolicy,
  createVerifiedOAuthUserCreationGuard,
} from "./googleOAuthPolicy.js";
import {
  createBetterAuthEmailCallbacks,
  createRuntimeMailConfigService,
} from "./mailConfig.js";
import { createRuntimeAuthRegistry } from "./runtimeAuthRegistry.js";
import { createRuntimeSocialProviderSettingsService } from "./socialProviderSettings.js";

export const authDB = new Pool({
  host: env.database.host,
  port: env.database.port,
  user: env.database.user,
  password: env.database.password,
  database: env.database.database,
  ssl:
    env.database.sslMode === "disable"
      ? false
      : { rejectUnauthorized: env.database.sslMode === "verify-full" },
  max: 10,
  connectionTimeoutMillis: 5_000,
  idleTimeoutMillis: 30_000,
});

export const mailConfigService = createRuntimeMailConfigService(authDB);
const mailCallbacks = createBetterAuthEmailCallbacks(mailConfigService);
const canonicalEmailPaths = new Set([
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

async function getRegistrationEnabled(): Promise<boolean> {
  try {
    const result = await authDB.query(
      "SELECT registration_enabled FROM get_auth_runtime_settings()",
    );
    return result.rows[0]?.registration_enabled === true;
  } catch (error) {
    console.error("[auth] Failed to query OAuth registration policy", error);
    return false;
  }
}

const authBaseOptions = {
  database: authDB,
  basePath: "/api/auth",
  appName: "IdeaBoard",
  ...(env.auth.secret ? { secret: env.auth.secret } : {}),
  ...(env.auth.baseURL ? { baseURL: env.auth.baseURL } : {}),
  trustedOrigins: env.auth.trustedOrigins,
  emailAndPassword: {
    enabled: true,
    autoSignIn: false,
    requireEmailVerification: true,
    minPasswordLength: 10,
    maxPasswordLength: 128,
    revokeSessionsOnPasswordReset: true,
    sendResetPassword: mailCallbacks.sendResetPassword,
  },
  emailVerification: {
    autoSignInAfterVerification: true,
    sendVerificationEmail: mailCallbacks.sendVerificationEmail,
  },
  session: {
    expiresIn: 60 * 60 * 24, // 24 hours — balances security with usability
    updateAge: 60 * 60, // refresh cookie every hour of active use
  },
  plugins: [
    emailOTP({
      otpLength: 6,
      expiresIn: 600,
      allowedAttempts: 5,
      storeOTP: "hashed",
      resendStrategy: "rotate",
      sendVerificationOnSignUp: true,
      overrideDefaultEmailVerification: true,
      changeEmail: { enabled: true, verifyCurrentEmail: false },
      rateLimit: { window: 60 * 60, max: 5 },
      sendVerificationOTP: async ({ email, otp, type }) => {
        await mailConfigService.send({
          to: email,
          subject:
            type === "forget-password"
              ? "重置你的 IdeaBoard Password"
              : "你的 IdeaBoard Email 验证码",
          html: `<p>你的验证码是：</p><p><strong>${otp}</strong></p>`,
        });
      },
    }),
    haveIBeenPwned({
      customPasswordCompromisedMessage: "该 Password 已出现在泄露数据中，请更换后重试",
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
        before: createVerifiedOAuthUserCreationGuard(getRegistrationEnabled),
      },
    },
  },
  hooks: {
    before: createAuthMiddleware(async (ctx) => {
      await applyGoogleOAuthRequestPolicy(ctx, getRegistrationEnabled);
      if (canonicalEmailPaths.has(ctx.path)) {
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

export const runtimeAuthRegistry = createRuntimeAuthRegistry(betterAuth, {
  baseOptions: authBaseOptions,
});
export const socialProviderSettingsService =
  createRuntimeSocialProviderSettingsService(
    authDB,
    env.auth.baseURL,
    env.secrets.encryptionKey,
    (config) => runtimeAuthRegistry.apply(config),
  );

// Migrations inspect this stable, provider-neutral instance. Runtime requests
// use runtimeAuthRegistry.current so credential changes apply without restart.
export const auth = runtimeAuthRegistry.current;
