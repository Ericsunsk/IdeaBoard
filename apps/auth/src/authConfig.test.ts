import assert from "node:assert/strict";
import { test } from "node:test";
import { auth } from "./auth.js";

test("native authentication security configuration stays explicit", () => {
  const options = auth.options;
  assert.equal(options.emailAndPassword?.enabled, true);
  assert.equal(options.emailAndPassword?.minPasswordLength, 10);
  assert.equal(options.emailAndPassword?.maxPasswordLength, 128);
  assert.equal(options.emailAndPassword?.requireEmailVerification, true);
  assert.equal(options.emailVerification?.autoSignInAfterVerification, true);

  const plugins = options.plugins ?? [];
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

  const hibp = plugins.find((plugin) => plugin.id === "have-i-been-pwned") as
    | { id: string; options?: { enabled?: boolean } }
    | undefined;
  assert.ok(hibp);
  assert.notEqual(hibp.options?.enabled, false);
});
