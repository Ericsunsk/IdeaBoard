import assert from "node:assert/strict";
import { test } from "node:test";
import { sessionIdentityFromAuthUser } from "./trustedIdentity.js";

test("session identity adapter normalizes a verified canonical email", () => {
  const identity = sessionIdentityFromAuthUser({
    id: " auth-1 ",
    email: " Alice@Example.com ",
    emailVerified: true,
    username: " Alice ",
  });

  assert.deepEqual(identity, {
    auth_user_id: "auth-1",
    email: "alice@example.com",
    username: "alice",
    claim_identity: "alice@example.com",
    claim_identity_kind: "canonical_email",
    claim_identity_verified: true,
  });
});

test("session identity adapter preserves fail-closed state for an untrusted claim", () => {
  const identity = sessionIdentityFromAuthUser({
    id: "auth-2",
    email: "not-an-email",
    emailVerified: true,
    name: "Alice",
  });

  assert.equal(identity.claim_identity_kind, "untrusted");
  assert.equal(identity.claim_identity_verified, false);
});
