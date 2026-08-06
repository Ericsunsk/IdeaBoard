import type {
  AuthRole,
  BanUserData,
  DeleteUserData,
  EnsureUserEnvelope,
  RevokeUserSessionsData,
  SetUserRoleData,
  UnbanUserData,
} from "../generated/auth-internal/types.gen.js";
import {
  normalizeAuthRole,
  normalizeDisplayName,
  normalizeOptionalPositiveInteger,
  normalizeOptionalText,
} from "../authUtils.js";
import {
  type TrustedIdentityProvisioner,
  type TrustedIdentityProvisioningOutcome,
} from "./trustedIdentity.js";

export type AuthSessionAPI = {
  getSession(input: { headers: Headers }): Promise<unknown>;
};

export type AuthIdentityMutationStore = {
  removeAuthUserCascade(authUserID: string): Promise<boolean>;
  updateAuthUserRole(authUserID: string, role: AuthRole): Promise<boolean>;
  updateAuthUserBanState(
    authUserID: string,
    banned: boolean,
    banReason: string | null,
    banExpiresIn: number | null,
  ): Promise<boolean>;
  deleteAuthUserSessions(authUserID: string): Promise<boolean>;
};

export type AuthInternalCommandFailureKind =
  "invalid_input" | "not_found" | "conflict" | "repository_failure";

export type AuthInternalCommandResult<TData> =
  | {
      ok: true;
      data: TData;
    }
  | {
      ok: false;
      kind: AuthInternalCommandFailureKind;
      message: string;
    };

export type EnsureIdentityCommandInput = {
  email?: unknown;
  displayName?: unknown;
  password?: unknown;
  allowExisting?: unknown;
  role?: unknown;
};

export type AuthUserIDCommandInput = {
  authUserID?: unknown;
};

export type BanIdentityCommandInput = AuthUserIDCommandInput & {
  banReason?: unknown;
  banExpiresIn?: unknown;
};

export type SetRoleCommandInput = AuthUserIDCommandInput & {
  role?: unknown;
};

export type TrustedIdentityData = EnsureUserEnvelope["data"];

export type AuthInternalCommands = {
  ensureIdentity(
    input: EnsureIdentityCommandInput,
  ): Promise<AuthInternalCommandResult<TrustedIdentityData>>;
  deleteIdentity(
    input: AuthUserIDCommandInput,
  ): Promise<AuthInternalCommandResult<DeleteUserData>>;
  banIdentity(
    input: BanIdentityCommandInput,
  ): Promise<AuthInternalCommandResult<BanUserData>>;
  unbanIdentity(
    input: AuthUserIDCommandInput,
  ): Promise<AuthInternalCommandResult<UnbanUserData>>;
  revokeSessions(
    input: AuthUserIDCommandInput,
  ): Promise<AuthInternalCommandResult<RevokeUserSessionsData>>;
  setRole(
    input: SetRoleCommandInput,
  ): Promise<AuthInternalCommandResult<SetUserRoleData>>;
};

export function createAuthInternalCommands(
  provisioner: TrustedIdentityProvisioner,
  mutationStore: AuthIdentityMutationStore,
): AuthInternalCommands {
  return {
    ensureIdentity: (input) => ensureIdentity(input, provisioner),
    deleteIdentity: (input) => deleteIdentity(input, mutationStore),
    banIdentity: (input) => banIdentity(input, mutationStore),
    unbanIdentity: (input) => unbanIdentity(input, mutationStore),
    revokeSessions: (input) => revokeSessions(input, mutationStore),
    setRole: (input) => setRole(input, mutationStore),
  };
}

async function ensureIdentity(
  input: EnsureIdentityCommandInput,
  provisioner: TrustedIdentityProvisioner,
): Promise<AuthInternalCommandResult<TrustedIdentityData>> {
  try {
    const outcome = await provisioner.provision(input);
    if (outcome.kind === "invalid") {
      return commandFailure("invalid_input", outcome.message);
    }
    if (outcome.kind === "conflict") {
      return commandFailure(
        "conflict",
        outcome.reason === "identity_mismatch"
          ? "existing identity conflicts with requested trusted user"
          : "auth identity already exists",
      );
    }
    return commandSuccess(trustedIdentityData(outcome));
  } catch (error) {
    return repositoryFailure(error, "failed to ensure user");
  }
}

async function deleteIdentity(
  input: AuthUserIDCommandInput,
  repository: AuthIdentityMutationStore,
) {
  return runAuthUserMutation(
    input,
    (authUserID) => repository.removeAuthUserCascade(authUserID),
    "failed to delete auth user",
    (authUserID) => ({
      deleted: true as const,
      auth_user_id: authUserID,
    }),
  );
}

async function banIdentity(
  input: BanIdentityCommandInput,
  repository: AuthIdentityMutationStore,
): Promise<AuthInternalCommandResult<BanUserData>> {
  const authUserID = validateAuthUserID(input.authUserID);
  if (!authUserID.ok) return authUserID.result;

  try {
    const banned = await repository.updateAuthUserBanState(
      authUserID.value,
      true,
      normalizeOptionalText(input.banReason, 500),
      normalizeOptionalPositiveInteger(input.banExpiresIn),
    );
    if (!banned) return commandFailure("not_found", "auth user not found");

    const revoked = await repository.deleteAuthUserSessions(authUserID.value);
    if (!revoked) {
      throw new Error("auth user sessions were not revoked after ban");
    }

    return commandSuccess({ auth_user_id: authUserID.value, banned: true });
  } catch (error) {
    return repositoryFailure(error, "failed to ban auth user");
  }
}

async function unbanIdentity(
  input: AuthUserIDCommandInput,
  repository: AuthIdentityMutationStore,
) {
  return runAuthUserMutation(
    input,
    (authUserID) =>
      repository.updateAuthUserBanState(authUserID, false, null, null),
    "failed to unban auth user",
    (authUserID) => ({ auth_user_id: authUserID, banned: false as const }),
  );
}

async function revokeSessions(
  input: AuthUserIDCommandInput,
  repository: AuthIdentityMutationStore,
) {
  return runAuthUserMutation(
    input,
    (authUserID) => repository.deleteAuthUserSessions(authUserID),
    "failed to revoke auth user sessions",
    (authUserID) => ({ auth_user_id: authUserID, revoked: true as const }),
  );
}

async function setRole(
  input: SetRoleCommandInput,
  repository: AuthIdentityMutationStore,
): Promise<AuthInternalCommandResult<SetUserRoleData>> {
  const authUserID = validateAuthUserID(input.authUserID);
  if (!authUserID.ok) return authUserID.result;

  const role = normalizeAuthRole(input.role);
  if (!role)
    return commandFailure("invalid_input", "role must be admin or user");

  try {
    const updated = await repository.updateAuthUserRole(authUserID.value, role);
    if (!updated) return commandFailure("not_found", "auth user not found");
    return commandSuccess({ auth_user_id: authUserID.value, role });
  } catch (error) {
    return repositoryFailure(error, "failed to update auth user role");
  }
}

async function runAuthUserMutation<TData>(
  input: AuthUserIDCommandInput,
  mutate: (authUserID: string) => Promise<boolean>,
  failureMessage: string,
  data: (authUserID: string) => TData,
): Promise<AuthInternalCommandResult<TData>> {
  const authUserID = validateAuthUserID(input.authUserID);
  if (!authUserID.ok) return authUserID.result;

  try {
    const updated = await mutate(authUserID.value);
    if (!updated) return commandFailure("not_found", "auth user not found");
    return commandSuccess(data(authUserID.value));
  } catch (error) {
    return repositoryFailure(error, failureMessage);
  }
}

function trustedIdentityData(
  outcome: Extract<
    TrustedIdentityProvisioningOutcome,
    { kind: "created" | "existing" }
  >,
): TrustedIdentityData {
  const { user, identity } = outcome;
  return {
    user: {
      auth_user_id: user.id,
      display_name: normalizeDisplayName(user.name || identity.displayName),
      email: identity.claimIdentity,
      claim_identity: identity.claimIdentity,
      claim_identity_kind: "canonical_email",
      claim_identity_verified: true,
      role: identity.role,
      existing: outcome.kind === "existing",
    },
  };
}

type AuthUserIDValidation =
  | { ok: true; value: string }
  | { ok: false; result: AuthInternalCommandResult<never> };

function validateAuthUserID(raw: unknown): AuthUserIDValidation {
  const value = String(raw ?? "").trim();
  if (!value) {
    return {
      ok: false,
      result: commandFailure("invalid_input", "auth user id is required"),
    };
  }
  if (value.length > 191) {
    return {
      ok: false,
      result: commandFailure("invalid_input", "auth user id is invalid"),
    };
  }
  return { ok: true, value };
}

function commandSuccess<TData>(data: TData): AuthInternalCommandResult<TData> {
  return { ok: true, data };
}

function commandFailure(
  kind: AuthInternalCommandFailureKind,
  message: string,
): AuthInternalCommandResult<never> {
  return { ok: false, kind, message };
}

function repositoryFailure(
  error: unknown,
  fallback: string,
): AuthInternalCommandResult<never> {
  console.error(`[auth] ${fallback}`, error);
  return commandFailure("repository_failure", fallback);
}
