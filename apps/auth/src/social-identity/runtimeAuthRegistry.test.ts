import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  createRuntimeAuthRegistry,
  type BetterAuthFactory,
} from "./runtimeAuthRegistry.js";

describe("RuntimeAuthRegistry", () => {
  it("rebuilds Better Auth with native Google and secure account linking", () => {
    const optionsSeen: Array<Record<string, unknown>> = [];
    const factory: BetterAuthFactory = (options) => {
      optionsSeen.push(options as unknown as Record<string, unknown>);
      return { marker: optionsSeen.length } as never;
    };
    const registry = createRuntimeAuthRegistry(factory, {
      baseOptions: { appName: "IdeaBoard" },
    });

    const initial = registry.current;
    registry.apply({
      google: {
        provider: "google",
        clientId: "example.apps.googleusercontent.com",
        clientSecret: "google-secret",
        scopes: ["openid", "email", "profile"],
      },
    });

    assert.notEqual(registry.current, initial);
    const options = optionsSeen[1] as {
      socialProviders?: { google?: Record<string, unknown> };
      account?: { accountLinking?: Record<string, unknown> };
    };
    assert.deepEqual(options.socialProviders?.google, {
      clientId: "example.apps.googleusercontent.com",
      clientSecret: "google-secret",
      accessType: "online",
      disableDefaultScope: true,
      disableImplicitSignUp: true,
      scope: ["openid", "email", "profile"],
      mapProfileToUser: options.socialProviders?.google?.mapProfileToUser,
    });
    const mapProfileToUser = options.socialProviders?.google?.mapProfileToUser;
    assert.equal(typeof mapProfileToUser, "function");
    assert.deepEqual(
      (mapProfileToUser as (profile: {
        email: string;
        email_verified: boolean;
        picture: string;
      }) => unknown)({
        email: "unverified@example.com",
        email_verified: false,
        picture: "https://lh3.googleusercontent.com/avatar",
      }),
      {
        email: null,
        emailVerified: false,
        image: "https://lh3.googleusercontent.com/avatar",
      },
    );
    assert.equal((options.account as { encryptOAuthTokens?: boolean }).encryptOAuthTokens, true);
    assert.deepEqual(options.account?.accountLinking, {
      enabled: true,
      disableImplicitLinking: false,
      requireLocalEmailVerified: true,
      trustedProviders: ["google"],
      allowDifferentEmails: false,
      updateUserInfoOnLink: false,
    });
  });

  it("removes Google without disturbing base authentication options", () => {
    const optionsSeen: Array<Record<string, unknown>> = [];
    const registry = createRuntimeAuthRegistry(
      ((options) => {
        optionsSeen.push(options as unknown as Record<string, unknown>);
        return { marker: optionsSeen.length } as never;
      }) as BetterAuthFactory,
      {
        baseOptions: {
          appName: "IdeaBoard",
          emailAndPassword: { enabled: true },
          plugins: [{ id: "email-otp" } as never],
        },
      },
    );

    registry.apply({
      google: {
        provider: "google",
        clientId: "example.apps.googleusercontent.com",
        clientSecret: "google-secret",
        scopes: ["openid", "email", "profile"],
      },
    });
    registry.apply({});

    const options = optionsSeen.at(-1) as {
      socialProviders?: unknown;
      emailAndPassword?: { enabled?: boolean };
      plugins?: Array<{ id?: string }>;
    };
    assert.equal(options.socialProviders, undefined);
    assert.equal(options.emailAndPassword?.enabled, true);
    assert.equal(options.plugins?.[0]?.id, "email-otp");
  });

  it("builds Google and Discord providers together without replacing either", () => {
    const optionsSeen: Array<Record<string, unknown>> = [];
    const registry = createRuntimeAuthRegistry(
      ((options) => {
        optionsSeen.push(options as unknown as Record<string, unknown>);
        return { marker: optionsSeen.length } as never;
      }) as BetterAuthFactory,
      { baseOptions: { appName: "IdeaBoard" } },
    );

    registry.apply({
      google: {
        provider: "google",
        clientId: "example.apps.googleusercontent.com",
        clientSecret: "google-secret",
        scopes: ["openid", "email", "profile"],
      },
      discord: {
        provider: "discord",
        clientId: "123456789012345678",
        clientSecret: "discord-secret",
        scopes: ["identify", "email"],
      },
    });

    const options = optionsSeen.at(-1) as {
      socialProviders?: Record<string, Record<string, unknown>>;
      account?: { accountLinking?: Record<string, unknown> };
    };
    assert.equal(options.socialProviders?.google?.clientId, "example.apps.googleusercontent.com");
    assert.equal(options.socialProviders?.discord?.clientId, "123456789012345678");
    assert.deepEqual(options.socialProviders?.discord?.scope, ["identify", "email"]);
    assert.deepEqual(options.account?.accountLinking?.trustedProviders, ["google", "discord"]);

    const mapProfileToUser = options.socialProviders?.discord?.mapProfileToUser;
    assert.equal(typeof mapProfileToUser, "function");
    assert.deepEqual(
      (mapProfileToUser as (profile: Record<string, unknown>) => unknown)({
        email: "unverified@example.com",
        verified: false,
        image_url: "https://cdn.discordapp.com/avatar.png",
        global_name: "Discord User",
      }),
      {
        email: null,
        emailVerified: false,
        image: "https://cdn.discordapp.com/avatar.png",
        name: "Discord User",
      },
    );
  });
});
