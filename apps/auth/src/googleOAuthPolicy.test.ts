import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  applyGoogleOAuthRequestPolicy,
  createVerifiedOAuthUserCreationGuard,
} from "./googleOAuthPolicy.js";

describe("Google OAuth policy", () => {
  it("removes caller scopes and derives sign-up intent from server policy", async () => {
    const context = {
      path: "/sign-in/social",
      body: {
        provider: "google",
        scopes: ["https://www.googleapis.com/auth/drive"],
        requestSignUp: false,
      },
    };

    await applyGoogleOAuthRequestPolicy(context, async () => true);

    assert.equal("scopes" in context.body, false);
    assert.equal(context.body.requestSignUp, true);
  });

  it("removes caller scopes from account linking without changing other providers", async () => {
    const google = {
      path: "/link-social",
      body: { provider: "google", scopes: ["drive"] },
    };
    const github = {
      path: "/sign-in/social",
      body: { provider: "github", scopes: ["repo"] },
    };

    await applyGoogleOAuthRequestPolicy(google, async () => true);
    await applyGoogleOAuthRequestPolicy(github, async () => true);

    assert.equal("scopes" in google.body, false);
    assert.deepEqual(github.body.scopes, ["repo"]);
  });

  it("checks current registration policy at verified OAuth user creation", async () => {
    const disabledGuard = createVerifiedOAuthUserCreationGuard(async () => false);
    const enabledGuard = createVerifiedOAuthUserCreationGuard(async () => true);

    assert.equal(await disabledGuard({ emailVerified: true }), false);
    assert.equal(await enabledGuard({ emailVerified: true }), undefined);
    assert.equal(await disabledGuard({ emailVerified: false }), undefined);
  });

  it("fails closed when registration policy cannot be read", async () => {
    const guard = createVerifiedOAuthUserCreationGuard(async () => {
      throw new Error("database unavailable");
    });
    const context = {
      path: "/sign-in/social",
      body: { provider: "google", requestSignUp: true },
    };

    await applyGoogleOAuthRequestPolicy(context, async () => {
      throw new Error("database unavailable");
    });

    assert.equal(context.body.requestSignUp, false);
    assert.equal(await guard({ emailVerified: true }), false);
  });
});
