import { betterAuth } from "better-auth";
import { createAuthMiddleware } from "better-auth/api";
import { admin, emailOTP, haveIBeenPwned, username } from "better-auth/plugins";
import { Pool } from "pg";
import { env } from "./env.js";
import { normalizeCanonicalEmail } from "./authUtils.js";
import {
  createBetterAuthEmailCallbacks,
  createRuntimeMailConfigService,
} from "./mailConfig.js";

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

export const auth = betterAuth({
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
    username({ minUsernameLength: 3, maxUsernameLength: 50 }),
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
  hooks: {
    before: createAuthMiddleware(async (ctx) => {
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
});
