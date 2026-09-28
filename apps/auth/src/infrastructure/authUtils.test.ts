import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  extractAuthUser,
  extractErrorMessage,
  getAuthSubpath,
  isDuplicateUserError,
  isValidCanonicalEmail,
  isRateLimitedAuthPath,
  isSignUpPath,
  normalizeAuthPath,
  normalizeAuthRole,
  normalizeCanonicalEmail,
  normalizeOptionalPositiveInteger,
  normalizeOptionalText,
  normalizeURLPath,
} from "./authUtils.js";

describe("auth route helpers", () => {
  it("normalizes URL paths and auth subpaths", () => {
    assert.equal(normalizeAuthPath("sign-in/email"), "/sign-in/email");
    assert.equal(
      normalizeURLPath("/api/auth/sign-in/email?next=/"),
      "/api/auth/sign-in/email",
    );
    assert.equal(getAuthSubpath("/api/auth"), "/");
    assert.equal(
      getAuthSubpath("/api/auth/sign-up/email?x=1"),
      "/sign-up/email",
    );
  });

  it("classifies guarded auth paths", () => {
    assert.equal(isSignUpPath("/sign-up/email"), true);
    assert.equal(isSignUpPath("/sign-in/email"), false);
    assert.equal(isRateLimitedAuthPath("/sign-in/email"), true);
    assert.equal(isRateLimitedAuthPath("/callback/github"), false);
  });
});

describe("auth input normalization", () => {
  it("normalizes roles", () => {
    assert.equal(normalizeAuthRole("ADMIN"), "admin");
    assert.equal(normalizeAuthRole(undefined), "user");
    assert.equal(normalizeAuthRole("owner"), null);
  });

  it("canonicalizes and validates Email identities", () => {
    assert.equal(
      normalizeCanonicalEmail(" Alice@Example.COM "),
      "alice@example.com",
    );
    assert.equal(isValidCanonicalEmail("alice@example.com"), true);
    assert.equal(isValidCanonicalEmail("alice@example"), false);
  });

  it("normalizes optional fields", () => {
    assert.equal(normalizeOptionalText("  hello world  ", 5), "hello");
    assert.equal(normalizeOptionalText("   ", 5), null);
    assert.equal(normalizeOptionalPositiveInteger("60"), 60);
    assert.equal(normalizeOptionalPositiveInteger("-1"), null);
    assert.equal(normalizeOptionalPositiveInteger("abc"), null);
  });
});

describe("auth payload helpers", () => {
  it("extracts auth users from supported Better Auth payload shapes", () => {
    assert.deepEqual(
      extractAuthUser({ id: "user_1", email: "a@example.com" }),
      {
        id: "user_1",
        email: "a@example.com",
      },
    );
    assert.deepEqual(extractAuthUser({ data: { user: { id: "user_2" } } }), {
      id: "user_2",
    });
    assert.equal(extractAuthUser({ data: { user: { id: "" } } }), null);
  });

  it("extracts display-safe error messages", () => {
    assert.equal(
      extractErrorMessage(new Error("failed"), "fallback"),
      "failed",
    );
    assert.equal(
      extractErrorMessage({ data: { msg: "bad request" } }, "fallback"),
      "bad request",
    );
    assert.equal(extractErrorMessage({}, "fallback"), "fallback");
  });

  it("detects duplicate-user errors", () => {
    assert.equal(isDuplicateUserError(new Error("user already exists")), true);
    assert.equal(
      isDuplicateUserError({ data: { message: "unique constraint failed" } }),
      true,
    );
    assert.equal(isDuplicateUserError(new Error("network unavailable")), false);
  });
});
