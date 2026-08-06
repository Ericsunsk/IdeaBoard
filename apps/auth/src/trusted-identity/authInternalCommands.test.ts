import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  createAuthInternalCommands,
  type AuthIdentityMutationStore,
} from "./authInternalCommands.js";
import {
  createTrustedIdentityProvisioner,
  type TrustedIdentityAuthAPI,
  type TrustedIdentityRepository,
} from "./trustedIdentity.js";

type TestIdentityStore = AuthIdentityMutationStore & TrustedIdentityRepository;

type CommandOverrides = {
  authApi?: Partial<TrustedIdentityAuthAPI>;
  repository?: Partial<TestIdentityStore>;
};

function createTestCommands(overrides: CommandOverrides = {}) {
  const authApi: TrustedIdentityAuthAPI = {
    signUpEmail: async ({ body }) => ({
      user: {
        id: "auth-created",
        name: body.name,
        email: body.email,
      },
    }),
    ...overrides.authApi,
  };
  const repository: TestIdentityStore = {
    findUserByEmail: async () => null,
    finalizeTrustedRegistrationIdentity: async () => true,
    removeAuthUserCascade: async () => true,
    updateAuthUserRole: async () => true,
    updateAuthUserBanState: async () => true,
    deleteAuthUserSessions: async () => true,
    ...overrides.repository,
  };
  return createAuthInternalCommands(
    createTrustedIdentityProvisioner(authApi, repository),
    repository,
  );
}

describe("AuthInternalCommands", () => {
  it("ensures a new trusted identity through the command Interface", async () => {
    const signups: unknown[] = [];
    const finalized: unknown[] = [];
    const commands = createTestCommands({
      authApi: {
        signUpEmail: async ({ body }) => {
          signups.push(body);
          return {
            user: {
              id: "auth-admin",
              name: body.name,
              email: body.email,
            },
          };
        },
      },
      repository: {
        finalizeTrustedRegistrationIdentity: async (
          authUserID,
          claimIdentity,
          role,
        ) => {
          finalized.push({ authUserID, claimIdentity, role });
          return true;
        },
      },
    });

    const result = await commands.ensureIdentity({
      email: "admin@example.com",
      displayName: "  管理员  ",
      password: "secret1234",
      role: "ADMIN",
    });

    assert.deepEqual(signups, [
      {
        email: "admin@example.com",
        password: "secret1234",
        name: "管理员",
      },
    ]);
    assert.deepEqual(finalized, [
      {
        authUserID: "auth-admin",
        claimIdentity: "admin@example.com",
        role: "admin",
      },
    ]);
    assert.deepEqual(result, {
      ok: true,
      data: {
        user: {
          auth_user_id: "auth-admin",
          display_name: "管理员",
          email: "admin@example.com",
          claim_identity: "admin@example.com",
          claim_identity_kind: "canonical_email",
          claim_identity_verified: true,
          role: "admin",
          existing: false,
        },
      },
    });
  });

  it("returns an existing identity only when allowExisting and every trusted claim match", async () => {
    const commands = createTestCommands({
      authApi: {
        signUpEmail: async () => {
          throw new Error("user already exists");
        },
      },
      repository: {
        findUserByEmail: async () => ({
          id: "auth-existing",
          name: "管理员",
          email: "admin@example.com",
          emailVerified: true,
          role: "admin",
        }),
      },
    });

    const result = await commands.ensureIdentity({
      email: "admin@example.com",
      displayName: "管理员",
      password: "secret1234",
      role: "admin",
      allowExisting: true,
    });

    assert.equal(result.ok, true);
    if (result.ok) assert.equal(result.data.user.existing, true);
  });

  it("classifies duplicate and mismatched trusted identities as conflicts", async () => {
    const duplicate = () => {
      throw new Error("user already exists");
    };
    const withoutReuse = createTestCommands({
      authApi: { signUpEmail: duplicate },
    });
    const mismatched = createTestCommands({
      authApi: { signUpEmail: duplicate },
      repository: {
        findUserByEmail: async () => ({
          id: "auth-attacker",
          name: "管理员",
          email: "admin@example.com",
          emailVerified: true,
          role: "user",
        }),
      },
    });

    assert.deepEqual(
      await withoutReuse.ensureIdentity({
        email: "admin@example.com",
        displayName: "管理员",
        password: "secret1234",
        role: "admin",
      }),
      { ok: false, kind: "conflict", message: "auth identity already exists" },
    );
    assert.deepEqual(
      await mismatched.ensureIdentity({
        email: "admin@example.com",
        displayName: "管理员",
        password: "secret1234",
        role: "admin",
        allowExisting: true,
      }),
      {
        ok: false,
        kind: "conflict",
        message: "existing identity conflicts with requested trusted user",
      },
    );
    assert.deepEqual(
      await withoutReuse.ensureIdentity({
        email: "admin@example.com",
        displayName: "管理员",
        password: "secret1234",
        role: "admin",
        allowExisting: "true",
      }),
      { ok: false, kind: "conflict", message: "auth identity already exists" },
    );
  });

  it("executes delete, ban, unban, revoke sessions, and set role with normalized input", async () => {
    const calls: unknown[] = [];
    const commands = createTestCommands({
      repository: {
        removeAuthUserCascade: async (authUserID) => {
          calls.push(["delete-cascade", authUserID]);
          return true;
        },
        updateAuthUserBanState: async (
          authUserID,
          banned,
          reason,
          expiresIn,
        ) => {
          calls.push(["ban-state", authUserID, banned, reason, expiresIn]);
          return true;
        },
        deleteAuthUserSessions: async (authUserID) => {
          calls.push(["sessions", authUserID]);
          return true;
        },
        updateAuthUserRole: async (authUserID, role) => {
          calls.push(["role", authUserID, role]);
          return true;
        },
      },
    });

    assert.deepEqual(
      await commands.deleteIdentity({ authUserID: " auth-delete " }),
      {
        ok: true,
        data: { deleted: true, auth_user_id: "auth-delete" },
      },
    );
    assert.deepEqual(
      await commands.banIdentity({
        authUserID: " auth-ban ",
        banReason: " abuse ",
        banExpiresIn: "3600",
      }),
      {
        ok: true,
        data: { auth_user_id: "auth-ban", banned: true },
      },
    );
    assert.deepEqual(
      await commands.unbanIdentity({ authUserID: " auth-unban " }),
      {
        ok: true,
        data: { auth_user_id: "auth-unban", banned: false },
      },
    );
    assert.deepEqual(
      await commands.revokeSessions({ authUserID: " auth-revoke " }),
      {
        ok: true,
        data: { auth_user_id: "auth-revoke", revoked: true },
      },
    );
    assert.deepEqual(
      await commands.setRole({ authUserID: " auth-role ", role: "ADMIN" }),
      {
        ok: true,
        data: { auth_user_id: "auth-role", role: "admin" },
      },
    );
    assert.deepEqual(calls, [
      ["delete-cascade", "auth-delete"],
      ["ban-state", "auth-ban", true, "abuse", 3600],
      ["sessions", "auth-ban"],
      ["ban-state", "auth-unban", false, null, null],
      ["sessions", "auth-revoke"],
      ["role", "auth-role", "admin"],
    ]);
  });

  it("rejects invalid inputs before calling dependencies", async () => {
    let called = false;
    const commands = createTestCommands({
      authApi: {
        signUpEmail: async () => {
          called = true;
          return {};
        },
      },
      repository: {
        removeAuthUserCascade: async () => {
          called = true;
          return true;
        },
        updateAuthUserRole: async () => {
          called = true;
          return true;
        },
        updateAuthUserBanState: async () => {
          called = true;
          return true;
        },
        deleteAuthUserSessions: async () => {
          called = true;
          return true;
        },
      },
    });

    assert.deepEqual(
      await commands.ensureIdentity({
        email: "admin@example.com",
        displayName: "",
        password: "secret1234",
        role: "admin",
      }),
      {
        ok: false,
        kind: "invalid_input",
        message: "display name is required",
      },
    );
    for (const password of ["123456789", "x".repeat(129)]) {
      assert.deepEqual(
        await commands.ensureIdentity({
          email: "admin@example.com",
          displayName: "管理员",
          password,
          role: "admin",
        }),
        {
          ok: false,
          kind: "invalid_input",
          message: "password length must be between 10 and 128",
        },
      );
    }
    for (const result of await Promise.all([
      commands.deleteIdentity({ authUserID: " " }),
      commands.banIdentity({ authUserID: " " }),
      commands.unbanIdentity({ authUserID: " " }),
      commands.revokeSessions({ authUserID: " " }),
      commands.setRole({ authUserID: " ", role: "admin" }),
    ])) {
      assert.deepEqual(result, {
        ok: false,
        kind: "invalid_input",
        message: "auth user id is required",
      });
    }
    assert.deepEqual(
      await commands.setRole({ authUserID: "auth-user", role: "owner" }),
      {
        ok: false,
        kind: "invalid_input",
        message: "role must be admin or user",
      },
    );
    assert.equal(called, false);
  });

  it("classifies missing users consistently and does not revoke sessions after a failed ban", async () => {
    let sessionsCalled = false;
    const commands = createTestCommands({
      repository: {
        removeAuthUserCascade: async () => false,
        updateAuthUserRole: async () => false,
        updateAuthUserBanState: async () => false,
        deleteAuthUserSessions: async () => {
          sessionsCalled = true;
          return false;
        },
      },
    });
    const expected = {
      ok: false,
      kind: "not_found",
      message: "auth user not found",
    };

    assert.deepEqual(
      await commands.deleteIdentity({ authUserID: "missing" }),
      expected,
    );
    assert.deepEqual(
      await commands.banIdentity({ authUserID: "missing" }),
      expected,
    );
    assert.equal(sessionsCalled, false);
    assert.deepEqual(
      await commands.unbanIdentity({ authUserID: "missing" }),
      expected,
    );
    assert.deepEqual(
      await commands.revokeSessions({ authUserID: "missing" }),
      expected,
    );
    assert.deepEqual(
      await commands.setRole({ authUserID: "missing", role: "user" }),
      expected,
    );
  });

  it("classifies dependency throws consistently as repository failures", async (t) => {
    t.mock.method(console, "error", () => {});
    const throwingCommands = createTestCommands({
      authApi: {
        signUpEmail: async () => {
          throw new Error("sign-up unavailable");
        },
      },
      repository: {
        removeAuthUserCascade: async () => {
          throw new Error("delete unavailable");
        },
        updateAuthUserRole: async () => {
          throw new Error("role unavailable");
        },
        updateAuthUserBanState: async (_id, banned) => {
          throw new Error(banned ? "ban unavailable" : "unban unavailable");
        },
        deleteAuthUserSessions: async (authUserID) => {
          throw new Error(`${authUserID} sessions unavailable`);
        },
      },
    });

    assert.deepEqual(
      await throwingCommands.ensureIdentity({
        email: "admin@example.com",
        displayName: "管理员",
        password: "secret1234",
        role: "admin",
      }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to ensure user",
      },
    );
    assert.deepEqual(
      await throwingCommands.deleteIdentity({ authUserID: "delete" }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to delete auth user",
      },
    );
    assert.deepEqual(
      await throwingCommands.banIdentity({ authUserID: "ban" }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to ban auth user",
      },
    );
    assert.deepEqual(
      await throwingCommands.unbanIdentity({ authUserID: "unban" }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to unban auth user",
      },
    );
    assert.deepEqual(
      await throwingCommands.revokeSessions({ authUserID: "revoke" }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to revoke auth user sessions",
      },
    );
    assert.deepEqual(
      await throwingCommands.setRole({ authUserID: "role", role: "admin" }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to update auth user role",
      },
    );
  });

  it("reports duplicate lookup throws as repository failures", async (t) => {
    t.mock.method(console, "error", () => {});
    const commands = createTestCommands({
      authApi: {
        signUpEmail: async () => {
          throw new Error("duplicate user");
        },
      },
      repository: {
        findUserByEmail: async () => {
          throw new Error("lookup unavailable");
        },
      },
    });

    assert.deepEqual(
      await commands.ensureIdentity({
        email: "admin@example.com",
        displayName: "管理员",
        password: "secret1234",
        role: "admin",
        allowExisting: true,
      }),
      {
        ok: false,
        kind: "repository_failure",
        message: "failed to ensure user",
      },
    );
  });

  it("retries the complete ban command when mandatory session revocation fails", async (t) => {
    t.mock.method(console, "error", () => {});
    const calls: string[] = [];
    let revokeAttempts = 0;
    const commands = createTestCommands({
      repository: {
        updateAuthUserBanState: async () => {
          calls.push("ban");
          return true;
        },
        deleteAuthUserSessions: async () => {
          calls.push("revoke");
          revokeAttempts++;
          if (revokeAttempts === 1)
            throw new Error("session store unavailable");
          return true;
        },
      },
    });

    assert.deepEqual(await commands.banIdentity({ authUserID: "auth-user" }), {
      ok: false,
      kind: "repository_failure",
      message: "failed to ban auth user",
    });
    assert.deepEqual(await commands.banIdentity({ authUserID: "auth-user" }), {
      ok: true,
      data: { auth_user_id: "auth-user", banned: true },
    });
    assert.deepEqual(calls, ["ban", "revoke", "ban", "revoke"]);
  });

  it("classifies a ban without session revocation as retryable repository failure", async (t) => {
    t.mock.method(console, "error", () => {});
    const commands = createTestCommands({
      repository: {
        updateAuthUserBanState: async () => true,
        deleteAuthUserSessions: async () => false,
      },
    });

    assert.deepEqual(await commands.banIdentity({ authUserID: "auth-user" }), {
      ok: false,
      kind: "repository_failure",
      message: "failed to ban auth user",
    });
  });
});
