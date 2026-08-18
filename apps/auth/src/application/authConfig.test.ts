import assert from "node:assert/strict";
import { test } from "node:test";
import { createAuthAdmissionPolicy } from "../admission/authAdmissionPolicy.js";
import { createBetterAuthOptions, createStaticBetterAuthOptions } from "./auth.js";
import { loadEnv } from "../infrastructure/env.js";

test("native authentication security configuration stays explicit", async (t) => {
  void t;
  const environment = loadEnv({
    NODE_ENV: "test",
    AUTH_INTERNAL_KEY: "test-internal-key",
    DB_SSLMODE: "disable",
  });
  const admissionPolicy = createAuthAdmissionPolicy({
    trustedOrigins: environment.auth.trustedOrigins,
    getSettings: async () => ({
      registrationEnabled: false,
      windowSeconds: 60,
      maxRequests: 30,
    }),
  });
  const options = createBetterAuthOptions({
    environment,
    database: {} as never,
    mailConfigService: { send: async () => {} },
    authAdmissionPolicy: admissionPolicy,
  });
  assert.equal(options.emailAndPassword?.enabled, true);
  assert.equal(options.emailAndPassword?.minPasswordLength, 10);
  assert.equal(options.emailAndPassword?.maxPasswordLength, 128);
  assert.equal(options.emailAndPassword?.requireEmailVerification, true);
  assert.equal(options.emailVerification?.autoSignInAfterVerification, true);
  assert.equal(options.emailAndPassword?.sendResetPassword, undefined);
  assert.equal(options.emailVerification?.sendVerificationEmail, undefined);

  const plugins = options.plugins ?? [];
  assert.equal(plugins.some((plugin) => plugin.id === "username"), false);
  const emailOTP = plugins.find((plugin) => plugin.id === "email-otp") as
    | {
        id: string;
        options?: {
          otpLength?: number;
          expiresIn?: number;
          allowedAttempts?: number;
          storeOTP?: string;
          resendStrategy?: string;
          sendVerificationOnSignUp?: boolean;
          changeEmail?: { enabled?: boolean };
          rateLimit?: { window?: number; max?: number };
        };
      }
    | undefined;
  assert.ok(emailOTP);
  assert.equal(emailOTP.options?.otpLength, 6);
  assert.equal(emailOTP.options?.expiresIn, 600);
  assert.equal(emailOTP.options?.allowedAttempts, 5);
  assert.equal(emailOTP.options?.storeOTP, "hashed");
  assert.equal(emailOTP.options?.resendStrategy, "rotate");
  assert.equal(emailOTP.options?.sendVerificationOnSignUp, true);
  assert.equal(emailOTP.options?.changeEmail?.enabled, true);
  assert.deepEqual(emailOTP.options?.rateLimit, { window: 3600, max: 5 });

  assert.equal(
    plugins.some((plugin) => plugin.id === "have-i-been-pwned"),
    false,
  );
});

test("static schema construction does not attach runtime provider configuration", () => {
  const environment = loadEnv({
    NODE_ENV: "test",
    AUTH_INTERNAL_KEY: "test-internal-key",
    DB_SSLMODE: "disable",
  });
  const options = createStaticBetterAuthOptions(environment, {} as never);

  assert.equal(options.socialProviders, undefined);
  assert.deepEqual(options.database, {});
  assert.ok(options.plugins?.some((plugin) => plugin.id === "email-otp"));
  assert.ok(options.plugins?.some((plugin) => plugin.id === "admin"));
});
