import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { describe, it } from "node:test";
import {
  createBetterAuthEmailCallbacks,
  createResendTransport,
  MailConfigError,
  MailConfigService,
  type MailConfigRepository,
  type MailReadiness,
  type MailSendInput,
  type StoredMailConfig,
} from "./mailConfig.js";

const encryptionKey = randomBytes(32).toString("base64");

describe("MailConfigService", () => {
  it("encrypts a new API key, never returns it, and resets readiness after configuration changes", async () => {
    const repository = createFakeRepository();
    const service = new MailConfigService(
      repository.port,
      { send: async () => {} },
      encryptionKey,
    );

    const summary = await service.update({
      apiKey: "re_super_secret",
      fromName: " IdeaBoard ",
      fromEmail: " Auth@Example.com ",
      replyTo: " Support@Example.com ",
      actorId: 7,
    });

    assert.equal(summary.readiness, "not_tested");
    assert.equal(summary.apiKeyConfigured, true);
    assert.equal(summary.fromEmail, "auth@example.com");
    assert.doesNotMatch(JSON.stringify(summary), /re_super_secret/);
    assert.ok(repository.value);
    assert.notEqual(repository.value.encryptedApiKey, "re_super_secret");
    assert.doesNotMatch(JSON.stringify(repository.audits), /re_super_secret/);
  });

  it("retains an existing API key when the update omits it", async () => {
    const repository = createFakeRepository();
    const service = new MailConfigService(
      repository.port,
      { send: async () => {} },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_initial",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });
    const encrypted = repository.value?.encryptedApiKey;

    await service.update({
      fromName: "IdeaBoard Mail",
      fromEmail: "auth@example.com",
      actorId: 2,
    });

    assert.equal(repository.value?.encryptedApiKey, encrypted);
    assert.equal(repository.value?.readiness, "not_tested");
  });

  it("keeps Ready for an identical save and invalidates it only when sender material changes", async () => {
    const repository = createFakeRepository();
    const service = new MailConfigService(
      repository.port,
      { send: async () => {} },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_initial",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });
    await service.sendTest({ recipient: "admin@example.com", actorId: 1 });

    const unchanged = await service.update({
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 2,
    });
    const changed = await service.update({
      fromName: "IdeaBoard Mail",
      fromEmail: "auth@example.com",
      actorId: 2,
    });

    assert.equal(unchanged.readiness, "ready");
    assert.equal(changed.readiness, "not_tested");
  });

  it("binds readiness to a successful test with the current encrypted configuration", async () => {
    const repository = createFakeRepository();
    const sends: Array<{ input: MailSendInput; apiKey: string }> = [];
    const service = new MailConfigService(
      repository.port,
      {
        send: async (input, apiKey) => {
          sends.push({ input, apiKey });
        },
      },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_runtime",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });

    await service.sendTest({ recipient: " Admin@Example.com ", actorId: 2 });

    assert.equal(repository.value?.readiness, "ready");
    assert.equal(sends[0]?.apiKey, "re_runtime");
    assert.equal(sends[0]?.input.to, "admin@example.com");
    assert.deepEqual(repository.audits.at(-1), {
      action: "test_mail_success",
      actorId: 2,
      metadata: { errorCode: null },
    });
  });

  it("records only a normalized provider error and marks the configuration unavailable", async () => {
    const repository = createFakeRepository();
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          throw new Error("api-key=re_do_not_log provider raw payload");
        },
      },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_runtime",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });

    await assert.rejects(
      () => service.sendTest({ recipient: "admin@example.com", actorId: 2 }),
      (error) =>
        error instanceof MailConfigError && error.code === "provider_error",
    );

    assert.equal(repository.value?.readiness, "unavailable");
    assert.equal(repository.value?.lastTestErrorCode, "provider_unavailable");
    assert.doesNotMatch(
      JSON.stringify(repository.audits),
      /re_do_not_log|raw payload/,
    );
  });

  it("fails closed until the current configuration has passed a real test", async () => {
    const repository = createFakeRepository();
    const service = new MailConfigService(
      repository.port,
      { send: async () => {} },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_runtime",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });

    await assert.rejects(
      () => service.send({ to: "user@example.com", subject: "Test", html: "" }),
      (error) => error instanceof MailConfigError && error.code === "unavailable",
    );
  });

  it("does not mark a newer configuration Ready when an older test finishes late", async () => {
    const repository = createFakeRepository();
    let service: MailConfigService;
    service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          await service.update({
            fromName: "IdeaBoard v2",
            fromEmail: "auth@example.com",
            actorId: 3,
          });
        },
      },
      encryptionKey,
    );
    await service.update({
      apiKey: "re_runtime",
      fromName: "IdeaBoard",
      fromEmail: "auth@example.com",
      actorId: 1,
    });

    await assert.rejects(
      () => service.sendTest({ recipient: "admin@example.com", actorId: 2 }),
      (error) =>
        error instanceof MailConfigError && error.code === "config_changed",
    );

    assert.equal(repository.value?.fromName, "IdeaBoard v2");
    assert.equal(repository.value?.readiness, "not_tested");
  });
});

it("Resend transport sends the supported HTTP contract without exposing credentials in the body", async () => {
  let request: { url: string; init?: RequestInit } | undefined;
  const fetchImpl = (async (
    url: string | URL | Request,
    init?: RequestInit,
  ) => {
    request = { url: String(url), init };
    return new Response(JSON.stringify({ id: "email-1" }), { status: 200 });
  }) as typeof fetch;
  await createResendTransport(fetchImpl).send(
    {
      from: "IdeaBoard <auth@example.com>",
      to: "admin@example.com",
      replyTo: "support@example.com",
      subject: "Test",
      html: "<p>Test</p>",
    },
    "re_secret",
  );

  assert.equal(request?.url, "https://api.resend.com/emails");
  assert.equal(
    new Headers(request?.init?.headers).get("authorization"),
    "Bearer re_secret",
  );
  assert.doesNotMatch(String(request?.init?.body), /re_secret/);
  assert.deepEqual(JSON.parse(String(request?.init?.body)), {
    from: "IdeaBoard <auth@example.com>",
    to: ["admin@example.com"],
    reply_to: "support@example.com",
    subject: "Test",
    html: "<p>Test</p>",
  });
});

it("Better Auth callbacks use the same runtime sender and escape action URLs", async () => {
  const sends: MailSendInput[] = [];
  const callbacks = createBetterAuthEmailCallbacks({
    send: async (input) => {
      sends.push(input);
    },
  });

  await callbacks.sendVerificationEmail({
    user: { email: "alice@example.com" },
    url: "https://example.com/verify?a=1&b=<x>",
  });
  await callbacks.sendResetPassword({
    user: { email: "alice@example.com" },
    url: "https://example.com/reset?a=1",
  });

  assert.equal(sends.length, 2);
  assert.match(sends[0]?.html || "", /a=1&amp;b=&lt;x&gt;/);
  assert.match(sends[1]?.subject || "", /Password/);
});

function createFakeRepository() {
  const state: {
    value: StoredMailConfig | null;
    audits: Array<{
      action: string;
      actorId: number;
      metadata?: Record<string, unknown>;
    }>;
  } = { value: null, audits: [] };
  const port: MailConfigRepository = {
    async get() {
      return state.value ? { ...state.value } : null;
    },
    async put(value, actorId, action) {
      state.value = {
        ...value,
        updatedAt: new Date("2026-08-05T10:00:00Z"),
        lastTestedAt: null,
        lastTestErrorCode: null,
      };
      state.audits.push({ action, actorId });
      return { ...state.value };
    },
    async markTest(
      readiness: Extract<MailReadiness, "ready" | "unavailable">,
      errorCode,
      actorId,
      configVersion,
    ) {
      assert.ok(state.value);
      if (state.value.configVersion !== configVersion) {
        state.audits.push({
          action: "test_mail_failure",
          actorId,
          metadata: { errorCode: "config_changed" },
        });
        return false;
      }
      state.value = {
        ...state.value,
        readiness,
        lastTestedAt: new Date("2026-08-05T10:01:00Z"),
        lastTestErrorCode: errorCode,
      };
      state.audits.push({
        action:
          readiness === "ready" ? "test_mail_success" : "test_mail_failure",
        actorId,
        metadata: { errorCode },
      });
      return true;
    },
    async clear(actorId) {
      state.value = null;
      state.audits.push({ action: "delete_mail_config", actorId });
    },
  };
  return {
    port,
    get value() {
      return state.value;
    },
    get audits() {
      return state.audits;
    },
  };
}
