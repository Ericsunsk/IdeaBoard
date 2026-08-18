import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createAuthAdmissionPolicy } from "./authAdmissionPolicy.js";

describe("AuthAdmissionPolicy", () => {
  it("denies Email credential registration before Mail and Rate Limit when registration is disabled", async () => {
    const calls: string[] = [];
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: ["https://app.example.com"],
      getSettings: async () => {
        calls.push("settings");
        return {
          registrationEnabled: false,
          windowSeconds: 60,
          maxRequests: 1,
        };
      },
      isMailReady: async () => {
        calls.push("mail");
        return true;
      },
      getSocialProviderReadiness: async () => {
        calls.push("provider");
        return true;
      },
    });

    const decision = await policy.admitRequest({
      method: "POST",
      path: "/sign-up/email",
      origin: "https://app.example.com",
      requestIP: "203.0.113.8",
    });

    assert.deepEqual(decision, {
      allowed: false,
      reason: "registration_disabled",
      corsOrigin: "https://app.example.com",
    });
    assert.deepEqual(calls, ["settings"]);
  });

  it("allows ready Email credential registration and applies Rate Limit after Mail readiness", async () => {
    const calls: string[] = [];
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: ["https://app.example.com"],
      getSettings: async () => {
        calls.push("settings");
        return {
          registrationEnabled: true,
          windowSeconds: 60,
          maxRequests: 1,
        };
      },
      isMailReady: async () => {
        calls.push("mail");
        return true;
      },
    });
    const request = {
      method: "POST",
      path: "/sign-up/email",
      origin: "",
      requestIP: "203.0.113.8",
    };

    assert.deepEqual(await policy.admitRequest(request), { allowed: true });
    const limited = await policy.admitRequest(request);
    assert.ok(!limited.allowed);
    assert.equal(limited.reason, "rate_limited");
    assert.ok((limited.retryAfterSeconds ?? 0) > 0);
    assert.deepEqual(calls, ["settings", "mail", "settings", "mail"]);
  });

  it("applies the shared registration and Mail decision to Email OTP user creation", async () => {
    let registrationEnabled = false;
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: [],
      getSettings: async () => ({
        registrationEnabled,
        windowSeconds: 60,
        maxRequests: 30,
      }),
      isMailReady: async () => true,
    });

    assert.deepEqual(await policy.admitRegistration("email_otp"), {
      allowed: false,
      reason: "registration_disabled",
    });

    registrationEnabled = true;
    assert.deepEqual(await policy.admitRegistration("email_otp"), {
      allowed: true,
    });
  });

  it("checks Mail readiness and client-IP Rate Limit at the Email OTP request entry", async () => {
    let isMailReady = false;
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: [],
      getSettings: async () => ({
        registrationEnabled: true,
        windowSeconds: 60,
        maxRequests: 1,
      }),
      isMailReady: async () => isMailReady,
    });
    const request = {
      method: "POST",
      path: "/email-otp/send-verification-otp",
      origin: "",
      requestIP: "",
      fallbackIP: "203.0.113.8",
    };

    assert.deepEqual(await policy.admitRequest(request), {
      allowed: false,
      reason: "mail_not_ready",
    });
    isMailReady = true;
    assert.deepEqual(await policy.admitRequest(request), { allowed: true });
    const limited = await policy.admitRequest(request);
    assert.ok(!limited.allowed);
    assert.equal(limited.reason, "rate_limited");
    assert.ok((limited.retryAfterSeconds ?? 0) > 0);
  });

  it("allows social provider creation only after Settings and Provider readiness", async () => {
    const calls: string[] = [];
    let providerReady = false;
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: [],
      getSettings: async () => {
        calls.push("settings");
        return {
          registrationEnabled: true,
          windowSeconds: 60,
          maxRequests: 30,
        };
      },
      getSocialProviderReadiness: async () => {
        calls.push("google");
        return providerReady;
      },
    });

    assert.deepEqual(await policy.admitRegistration("google_oauth"), {
      allowed: false,
      reason: "provider_not_ready",
    });
    providerReady = true;
    assert.deepEqual(await policy.admitRegistration("google_oauth"), {
      allowed: true,
    });
    assert.deepEqual(calls, ["settings", "google", "settings", "google"]);
  });

  it("owns Origin and preflight ordering before dependency reads", async () => {
    let settingsReads = 0;
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: ["https://app.example.com"],
      getSettings: async () => {
        settingsReads += 1;
        return {
          registrationEnabled: true,
          windowSeconds: 60,
          maxRequests: 30,
        };
      },
    });

    assert.deepEqual(
      await policy.admitRequest({
        method: "POST",
        path: "/sign-in/email",
        origin: "https://evil.example.com",
        requestIP: "203.0.113.8",
      }),
      { allowed: false, reason: "origin_not_allowed" },
    );
    assert.deepEqual(
      await policy.admitRequest({
        method: "OPTIONS",
        path: "/sign-in/email",
        origin: "https://app.example.com",
        requestIP: "203.0.113.8",
      }),
      {
        allowed: true,
        corsOrigin: "https://app.example.com",
        preflight: true,
      },
    );
    assert.equal(settingsReads, 0);
  });

  it("keeps sign-in available when registration is disabled", async () => {
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: [],
      getSettings: async () => ({
        registrationEnabled: false,
        windowSeconds: 60,
        maxRequests: 30,
      }),
    });

    assert.deepEqual(
      await policy.admitRequest({
        method: "POST",
        path: "/sign-in/email",
        origin: "",
        requestIP: "203.0.113.8",
      }),
      { allowed: true },
    );
  });

  it("fails closed when Settings cannot be read", async () => {
    const policy = createAuthAdmissionPolicy({
      trustedOrigins: [],
      onDependencyError: () => {},
      getSettings: async () => {
        throw new Error("database unavailable");
      },
    });

    assert.deepEqual(
      await policy.admitRequest({
        method: "POST",
        path: "/sign-in/email",
        origin: "",
        requestIP: "203.0.113.8",
      }),
      { allowed: false, reason: "admission_unavailable" },
    );
    assert.deepEqual(await policy.admitRegistration("google_oauth"), {
      allowed: false,
      reason: "admission_unavailable",
    });
  });
});
