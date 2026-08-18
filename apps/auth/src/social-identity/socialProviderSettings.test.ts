import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createSecretCipher } from "../infrastructure/secretCipher.js";
import {
  DISCORD_PROVIDER,
  GOOGLE_PROVIDER,
  SocialProviderSettingsError,
  SocialProviderSettingsService,
  type SocialProviderSettingsRepository,
  type StoredSocialProviderSettings,
} from "./socialProviderSettings.js";

const secretCipher = createSecretCipher(
  Buffer.alloc(32, 7).toString("base64"),
);

describe("SocialProviderSettingsService", () => {
  it("keeps an unconfigured provider disabled and exposes the fixed callback", async () => {
    const service = new SocialProviderSettingsService(
      memoryRepository(),
      secretCipher,
      "https://api.ideaboard.art",
    );

    assert.deepEqual(await service.getSummary(GOOGLE_PROVIDER), {
      provider: "google",
      enabled: false,
      configured: false,
      available: false,
      clientId: "",
      clientSecretConfigured: false,
      scopes: ["openid", "email", "profile"],
      configVersion: 0,
      callbackURL: "https://api.ideaboard.art/api/auth/callback/google",
      updatedAt: null,
    });
    assert.equal(await service.resolve(GOOGLE_PROVIDER), null);
    assert.deepEqual(await service.getSummary(DISCORD_PROVIDER), {
      provider: "discord",
      enabled: false,
      configured: false,
      available: false,
      clientId: "",
      clientSecretConfigured: false,
      scopes: ["identify", "email"],
      configVersion: 0,
      callbackURL: "https://api.ideaboard.art/api/auth/callback/discord",
      updatedAt: null,
    });
  });

  it("supports Discord credentials with a fixed minimal scope", async () => {
    const repository = memoryRepository();
    const applied: string[][] = [];
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
      async (configs) => {
        applied.push(Object.keys(configs).sort());
      },
    );

    const summary = await service.update(DISCORD_PROVIDER, {
      enabled: true,
      clientId: "123456789012345678",
      clientSecret: "discord-secret",
    });

    assert.equal(summary.available, true);
    assert.deepEqual(await service.resolve(DISCORD_PROVIDER), {
      provider: "discord",
      clientId: "123456789012345678",
      clientSecret: "discord-secret",
      scopes: ["identify", "email"],
    });
    assert.deepEqual(await service.getCapabilities(), {
      social: { google: false, discord: true },
    });
    assert.deepEqual(applied, [["discord"]]);
  });

  it("encrypts the secret, preserves it on blank updates, and resolves runtime config", async () => {
    const repository = memoryRepository();
    const applied: Array<string | null> = [];
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
      async (configs) => {
        applied.push(configs.google?.clientId ?? null);
      },
    );

    const first = await service.update(GOOGLE_PROVIDER, {
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
    });
    const storedAfterFirst = await repository.get(GOOGLE_PROVIDER);

    assert.equal(first.available, true);
    assert.equal(first.clientSecretConfigured, true);
    assert.notEqual(storedAfterFirst?.encryptedClientSecret, "google-secret");
    assert.equal(
      JSON.stringify(first).includes("google-secret"),
      false,
      "summary must never expose plaintext secrets",
    );
    assert.deepEqual(await service.resolve(GOOGLE_PROVIDER), {
      provider: "google",
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
      scopes: ["openid", "email", "profile"],
    });

    await service.update(GOOGLE_PROVIDER, {
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "",
    });
    const storedAfterBlankUpdate = await repository.get(GOOGLE_PROVIDER);
    assert.equal(
      storedAfterBlankUpdate?.encryptedClientSecret,
      storedAfterFirst?.encryptedClientSecret,
    );
    assert.deepEqual(applied, ["example.apps.googleusercontent.com"]);
  });

  it("serializes concurrent updates so the runtime matches the latest saved state", async () => {
    const repository = memoryRepository();
    const applied: Array<string | null> = [];
    let releaseFirstApply!: () => void;
    const firstApplyGate = new Promise<void>((resolve) => {
      releaseFirstApply = resolve;
    });
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
      async (configs) => {
        applied.push(configs.google?.clientId ?? null);
        if (configs.google) await firstApplyGate;
      },
    );

    const enable = service.update(GOOGLE_PROVIDER, {
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
    });
    while (applied.length === 0) await Promise.resolve();
    const disable = service.update(GOOGLE_PROVIDER, {
      enabled: false,
      clientId: "example.apps.googleusercontent.com",
    });

    await Promise.resolve();
    assert.deepEqual(applied, ["example.apps.googleusercontent.com"]);
    releaseFirstApply();
    await Promise.all([enable, disable]);

    assert.deepEqual(applied, ["example.apps.googleusercontent.com", null]);
    assert.equal((await repository.get(GOOGLE_PROVIDER))?.enabled, false);
    assert.equal((await service.getCapabilities()).social.google, false);
  });

  it("requires complete credentials before enabling", async () => {
    const service = new SocialProviderSettingsService(
      memoryRepository(),
      secretCipher,
      "https://api.ideaboard.art",
    );

    await assert.rejects(
      () =>
        service.update(GOOGLE_PROVIDER, {
          enabled: true,
          clientId: "example.apps.googleusercontent.com",
        }),
      (error: unknown) =>
        error instanceof SocialProviderSettingsError &&
        error.code === "incomplete_configuration",
    );
  });

  it("requires an enabled provider to be disabled before changing client ID", async () => {
    const repository = memoryRepository({
      provider: GOOGLE_PROVIDER,
      enabled: true,
      clientId: "old.apps.googleusercontent.com",
      encryptedClientSecret: "ciphertext",
      scopes: ["openid", "email", "profile"],
      configVersion: 1,
      updatedAt: new Date("2026-08-06T00:00:00.000Z"),
    });
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
    );

    await assert.rejects(
      () =>
        service.update(GOOGLE_PROVIDER, {
          enabled: true,
          clientId: "new.apps.googleusercontent.com",
          clientSecret: "replacement-secret",
        }),
      (error: unknown) =>
        error instanceof SocialProviderSettingsError &&
        error.code === "disable_before_client_id_change",
    );
  });

  it("fails closed when stored credentials cannot be decrypted", async () => {
    const repository = memoryRepository({
      provider: GOOGLE_PROVIDER,
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      encryptedClientSecret: "not-valid-ciphertext",
      scopes: ["openid", "email", "profile"],
      configVersion: 1,
      updatedAt: new Date("2026-08-06T00:00:00.000Z"),
    });
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
    );

    assert.equal(await service.resolve(GOOGLE_PROVIDER), null);
    assert.equal((await service.getSummary(GOOGLE_PROVIDER)).available, false);
  });

  it("does not persist enabled when the retained secret cannot be decrypted", async () => {
    const repository = memoryRepository({
      provider: GOOGLE_PROVIDER,
      enabled: false,
      clientId: "example.apps.googleusercontent.com",
      encryptedClientSecret: "not-valid-ciphertext",
      scopes: ["openid", "email", "profile"],
      configVersion: 1,
      updatedAt: new Date("2026-08-06T00:00:00.000Z"),
    });
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
    );

    await assert.rejects(
      () =>
        service.update(GOOGLE_PROVIDER, {
          enabled: true,
          clientId: "example.apps.googleusercontent.com",
        }),
      (error: unknown) =>
        error instanceof SocialProviderSettingsError &&
        error.code === "encryption_unavailable",
    );
    assert.equal((await repository.get(GOOGLE_PROVIDER))?.enabled, false);
  });

  it("fails closed when stored scopes differ from the code allowlist", async () => {
    const repository = memoryRepository();
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
    );
    await service.update(GOOGLE_PROVIDER, {
      enabled: true,
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
    });
    const stored = await repository.get(GOOGLE_PROVIDER);
    assert.ok(stored);
    stored.scopes = [...stored.scopes, "https://www.googleapis.com/auth/drive"];
    const corruptedRepository = memoryRepository(stored);
    const corruptedService = new SocialProviderSettingsService(
      corruptedRepository,
      secretCipher,
      "https://api.ideaboard.art",
    );

    assert.equal(await corruptedService.resolve(GOOGLE_PROVIDER), null);
    assert.equal(
      (await corruptedService.getCapabilities()).social.google,
      false,
    );
  });

  it("initializes Google as disabled when the settings repository is unavailable", async () => {
    const applied: Array<string | null> = [];
    const repository: SocialProviderSettingsRepository = {
      async get() {
        throw new Error("database unavailable");
      },
      async put() {
        throw new Error("database unavailable");
      },
    };
    const service = new SocialProviderSettingsService(
      repository,
      secretCipher,
      "https://api.ideaboard.art",
      async (configs) => {
        applied.push(configs.google?.provider ?? null);
      },
    );

    await service.initialize();

    assert.deepEqual(applied, [null]);
    assert.deepEqual(await service.getCapabilities(), {
      social: { google: false, discord: false },
    });
  });

  it("rejects providers outside the code allowlist", async () => {
    const service = new SocialProviderSettingsService(
      memoryRepository(),
      secretCipher,
      "https://api.ideaboard.art",
    );

    await assert.rejects(
      () => service.getSummary("custom"),
      (error: unknown) =>
        error instanceof SocialProviderSettingsError &&
        error.code === "unsupported_provider",
    );
  });
});

function memoryRepository(
  initial: StoredSocialProviderSettings | null = null,
): SocialProviderSettingsRepository {
  let current = initial;
  return {
    async get(provider) {
      return current?.provider === provider ? structuredClone(current) : null;
    },
    async put(value) {
      current = {
        ...value,
        configVersion: (current?.configVersion ?? 0) + 1,
        updatedAt: new Date("2026-08-06T00:00:00.000Z"),
      };
      return structuredClone(current);
    },
  };
}
