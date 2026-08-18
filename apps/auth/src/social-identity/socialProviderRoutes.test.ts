import assert from "node:assert/strict";
import { describe, it } from "node:test";
import Fastify from "fastify";
import type {
  SocialProviderSettingsInput,
  SocialProviderSettingsSummary,
} from "./socialProviderSettings.js";
import { registerSocialProviderRoutes } from "./socialProviderRoutes.js";

const internalKey = "social-provider-internal-key";
const summary: SocialProviderSettingsSummary = {
  provider: "google",
  enabled: true,
  configured: true,
  available: true,
  clientId: "example.apps.googleusercontent.com",
  clientSecretConfigured: true,
  scopes: ["openid", "email", "profile"],
  configVersion: 1,
  callbackURL: "https://api.ideaboard.art/api/auth/callback/google",
  updatedAt: "2026-08-06T00:00:00.000Z",
};

describe("social provider HTTP contracts", () => {
  it("protects internal settings routes and never returns the secret", async (t) => {
    let updateInput: SocialProviderSettingsInput | undefined;
    const app = createApp({
      update: async (_provider, input) => {
        updateInput = input;
        return summary;
      },
    });
    t.after(() => app.close());

    const unauthorized = await app.inject({
      method: "GET",
      url: "/internal/social-providers/google",
    });
    assert.equal(unauthorized.statusCode, 401);

    const getResponse = await app.inject({
      method: "GET",
      url: "/internal/social-providers/google",
      headers: authHeaders(),
    });
    assert.equal(getResponse.statusCode, 200);
    assert.equal(getResponse.headers["cache-control"], "no-store");
    assert.equal(getResponse.json().data.client_secret_configured, true);
    assert.doesNotMatch(getResponse.body, /google-secret|client_secret"/);

    const putResponse = await app.inject({
      method: "PUT",
      url: "/internal/social-providers/google",
      headers: authHeaders(),
      payload: {
        enabled: true,
        client_id: "example.apps.googleusercontent.com",
        client_secret: "google-secret",
      },
    });
    assert.equal(putResponse.statusCode, 200);
    assert.deepEqual(updateInput, {
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
    });
    assert.doesNotMatch(putResponse.body, /google-secret/);
  });

  it("publishes a no-store runtime capability without credentials", async (t) => {
    const app = createApp({}, ["https://app.ideaboard.art"]);
    t.after(() => app.close());

    const response = await app.inject({
      method: "GET",
      url: "/api/auth/capabilities",
    });

    assert.equal(response.statusCode, 200);
    assert.equal(response.headers["cache-control"], "no-store");
    assert.deepEqual(response.json().data, {
      social: { google: true, discord: false },
    });
    assert.doesNotMatch(response.body, /client|secret|scope/i);

    const allowed = await app.inject({
      method: "GET",
      url: "/api/auth/capabilities",
      headers: { origin: "https://app.ideaboard.art" },
    });
    assert.equal(
      allowed.headers["access-control-allow-origin"],
      "https://app.ideaboard.art",
    );

    const denied = await app.inject({
      method: "GET",
      url: "/api/auth/capabilities",
      headers: { origin: "https://evil.example" },
    });
    assert.equal(denied.statusCode, 403);
  });
});

function createApp(
  overrides: Partial<{
    getSummary: (
      provider: string,
    ) => Promise<SocialProviderSettingsSummary>;
    update: (
      provider: string,
      input: SocialProviderSettingsInput,
    ) => Promise<SocialProviderSettingsSummary>;
    getCapabilities: () => Promise<{ social: { google: boolean; discord: boolean } }>;
  }> = {},
  trustedOrigins: string[] = [],
) {
  const app = Fastify();
  registerSocialProviderRoutes(app, {
    internalKey,
    trustedOrigins,
    getPublicCapabilities:
      overrides.getCapabilities ??
      (async () => ({ social: { google: true, discord: false } })),
    service: {
      getSummary: async () => summary,
      update: async () => summary,
      ...overrides,
    },
  });
  return app;
}

function authHeaders() {
  return { "x-internal-key": internalKey };
}
