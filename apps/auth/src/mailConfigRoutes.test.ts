import assert from "node:assert/strict";
import { describe, it } from "node:test";
import Fastify from "fastify";
import type {
  MailConfigInput,
  MailConfigSummary,
  MailTestInput,
} from "./mailConfig.js";
import { registerMailConfigRoutes } from "./mailConfigRoutes.js";

const internalKey = "mail-config-internal-key";
const summary: MailConfigSummary = {
  configured: true,
  apiKeyConfigured: true,
  fromName: "IdeaBoard",
  fromEmail: "auth@example.com",
  replyTo: "support@example.com",
  configVersion: 1,
  readiness: "not_tested",
  updatedAt: "2026-08-05T10:00:00.000Z",
  lastTestedAt: null,
  lastTestErrorCode: null,
};

describe("mail config internal HTTP contract", () => {
  it("rejects requests without the internal credential", async (t) => {
    const app = createApp();
    t.after(() => app.close());

    const response = await app.inject({
      method: "GET",
      url: "/internal/mail/config",
    });

    assert.equal(response.statusCode, 401);
    assert.equal(response.json().msg, "unauthorized internal request");
  });

  it("returns only a no-store configuration summary and accepts audited updates", async (t) => {
    let updateInput: MailConfigInput | undefined;
    const app = createApp({
      update: async (input) => {
        updateInput = input;
        return summary;
      },
    });
    t.after(() => app.close());

    const getResponse = await app.inject({
      method: "GET",
      url: "/internal/mail/config",
      headers: authHeaders(),
    });
    assert.equal(getResponse.statusCode, 200);
    assert.equal(getResponse.headers["cache-control"], "no-store");
    assert.equal(getResponse.json().data.api_key_configured, true);
    assert.doesNotMatch(getResponse.body, /api_key"|re_/);

    const updateResponse = await app.inject({
      method: "PUT",
      url: "/internal/mail/config",
      headers: authHeaders(),
      payload: {
        api_key: "re_secret",
        from_name: "IdeaBoard",
        from_email: "auth@example.com",
        reply_to: "support@example.com",
        actor_id: 19,
      },
    });
    assert.equal(updateResponse.statusCode, 200);
    assert.deepEqual(updateInput, {
      apiKey: "re_secret",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      replyTo: "support@example.com",
      actorId: 19,
    });
    assert.doesNotMatch(updateResponse.body, /re_secret/);
  });

  it("forwards the Admin actor to test and delete audit commands", async (t) => {
    let testInput: MailTestInput | undefined;
    let deletedBy = 0;
    const app = createApp({
      sendTest: async (input) => {
        testInput = input;
      },
      clear: async (actorId) => {
        deletedBy = actorId;
      },
    });
    t.after(() => app.close());

    const testResponse = await app.inject({
      method: "POST",
      url: "/internal/mail/config/test",
      headers: authHeaders(),
      payload: { recipient: "admin@example.com", actor_id: 23 },
    });
    const deleteResponse = await app.inject({
      method: "DELETE",
      url: "/internal/mail/config",
      headers: authHeaders(),
      payload: { actor_id: 23 },
    });

    assert.equal(testResponse.statusCode, 200);
    assert.equal(deleteResponse.statusCode, 200);
    assert.deepEqual(testInput, {
      recipient: "admin@example.com",
      actorId: 23,
    });
    assert.equal(deletedBy, 23);
  });
});

function createApp(
  overrides: Partial<{
    getSummary: () => Promise<MailConfigSummary>;
    update: (input: MailConfigInput) => Promise<MailConfigSummary>;
    clear: (actorId: number) => Promise<void>;
    sendTest: (input: MailTestInput) => Promise<void>;
  }> = {},
) {
  const app = Fastify();
  registerMailConfigRoutes(app, {
    internalKey,
    service: {
      getSummary: async () => summary,
      update: async () => summary,
      clear: async () => {},
      sendTest: async () => {},
      ...overrides,
    },
  });
  return app;
}

function authHeaders() {
  return { "x-internal-key": internalKey };
}
