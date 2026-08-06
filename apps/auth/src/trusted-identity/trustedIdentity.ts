import {
  extractAuthUser,
  isDuplicateUserError,
  isValidCanonicalEmail,
  normalizeAuthRole,
  normalizeCanonicalEmail,
  normalizeAvatarURL,
  normalizeDisplayName,
  type InternalUserShape,
} from "../infrastructure/authUtils.js";
import type { AuthRole } from "../generated/auth-internal/types.gen.js";

export type TrustedIdentityAuthAPI = {
  signUpEmail(input: {
    body: {
      email: string;
      password: string;
      name: string;
    };
  }): Promise<unknown>;
};

export type VerifiedSessionIdentity = {
  auth_user_id: string;
  email: string;
  display_name: string;
  avatar: string;
  claim_identity: string;
  claim_identity_kind: "canonical_email" | "untrusted";
  claim_identity_verified: boolean;
};

export function sessionIdentityFromAuthUser(
  user: InternalUserShape,
): VerifiedSessionIdentity {
  const displayName = normalizeDisplayName(user.name);
  const claimIdentity = normalizeCanonicalEmail(user.email);
  const isTrustedClaimIdentity =
    Boolean(user.emailVerified) && isValidCanonicalEmail(claimIdentity);

  return {
    auth_user_id: user.id.trim(),
    email: claimIdentity,
    display_name: displayName,
    avatar: normalizeAvatarURL(user.image),
    claim_identity: claimIdentity,
    claim_identity_kind: isTrustedClaimIdentity
      ? "canonical_email"
      : "untrusted",
    claim_identity_verified: isTrustedClaimIdentity,
  };
}

export type TrustedIdentityRepository = {
  findUserByEmail(email: string): Promise<InternalUserShape | null>;
  finalizeTrustedRegistrationIdentity(
    authUserID: string,
    claimIdentity: string,
    role: AuthRole,
  ): Promise<boolean>;
  removeAuthUserCascade(authUserID: string): Promise<boolean>;
};

export type TrustedIdentityProvisioningInput = {
  email?: unknown;
  displayName?: unknown;
  password?: unknown;
  role?: unknown;
  allowExisting?: unknown;
};

export type TrustedIdentityProvisioningOutcome =
  | {
      kind: "created" | "existing";
      user: InternalUserShape;
      identity: {
        claimIdentity: string;
        displayName: string;
        role: AuthRole;
      };
    }
  | {
      kind: "conflict";
      reason: "already_exists" | "identity_mismatch";
    }
  | {
      kind: "invalid";
      message: string;
    };

export type TrustedIdentityProvisioner = {
  provision(
    input: TrustedIdentityProvisioningInput,
  ): Promise<TrustedIdentityProvisioningOutcome>;
};

export class TrustedIdentityProvisioningError extends Error {
  constructor(
    readonly code: "dependency_failure" | "reconciliation_required",
    message: string,
    readonly authUserID?: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "TrustedIdentityProvisioningError";
  }
}

export function createTrustedIdentityProvisioner(
  authApi: TrustedIdentityAuthAPI,
  repository: TrustedIdentityRepository,
): TrustedIdentityProvisioner {
  return {
    async provision(input) {
      const claimIdentity = normalizeCanonicalEmail(input.email);
      const displayName = normalizeDisplayName(input.displayName);
      const password = String(input.password || "");
      const role = normalizeAuthRole(input.role);
      if (!isValidCanonicalEmail(claimIdentity)) {
        return { kind: "invalid", message: "a valid email is required" };
      }
      if (!displayName) {
        return { kind: "invalid", message: "display name is required" };
      }
      if (password.length < 10 || password.length > 128) {
        return {
          kind: "invalid",
          message: "password length must be between 10 and 128",
        };
      }
      if (!role) {
        return {
          kind: "invalid",
          message: "role must be admin or user",
        };
      }

      const identity = { claimIdentity, displayName, role };
      let createdUserID = "";
      try {
        let created: unknown;
        try {
          created = await authApi.signUpEmail({
            body: { email: claimIdentity, password, name: displayName },
          });
        } catch (error) {
          if (isDuplicateUserError(error)) {
            return resolveExistingIdentity(
              repository,
              claimIdentity,
              role,
              identity,
              input.allowExisting === true,
            );
          }
          throw error;
        }

        const user = extractAuthUser(created);
        if (!user) {
          let recovered: InternalUserShape | null;
          try {
            recovered = await repository.findUserByEmail(claimIdentity);
          } catch (lookupError) {
            throw new TrustedIdentityProvisioningError(
              "reconciliation_required",
              "trusted identity sign-up returned no user and recovery failed",
              undefined,
              { cause: lookupError },
            );
          }
          if (!recovered?.id) {
            throw new TrustedIdentityProvisioningError(
              "reconciliation_required",
              "trusted identity sign-up returned no recoverable user",
            );
          }
          createdUserID = recovered.id;
          throw new Error("auth user creation returned an empty payload");
        }

        createdUserID = user.id;
        if (
          !(await repository.finalizeTrustedRegistrationIdentity(
            user.id,
            claimIdentity,
            role,
          ))
        ) {
          throw new Error(
            "failed to finalize trusted canonical registration identity",
          );
        }
        return { kind: "created", user, identity };
      } catch (error) {
        if (
          error instanceof TrustedIdentityProvisioningError &&
          error.code === "reconciliation_required" &&
          !createdUserID
        ) {
          throw error;
        }
        if (createdUserID) {
          try {
            // Missing during cleanup is already the desired converged state.
            await repository.removeAuthUserCascade(createdUserID);
          } catch (cleanupError) {
            throw new TrustedIdentityProvisioningError(
              "reconciliation_required",
              "trusted identity provisioning failed and cleanup requires operator attention",
              createdUserID,
              { cause: new AggregateError([error, cleanupError]) },
            );
          }
        }
        if (error instanceof TrustedIdentityProvisioningError) throw error;
        throw new TrustedIdentityProvisioningError(
          "dependency_failure",
          "trusted identity provisioning failed",
          createdUserID || undefined,
          { cause: error },
        );
      }
    },
  };
}

async function resolveExistingIdentity(
  repository: TrustedIdentityRepository,
  claimIdentity: string,
  role: AuthRole,
  identity: {
    claimIdentity: string;
    displayName: string;
    role: AuthRole;
  },
  allowExisting: boolean,
): Promise<TrustedIdentityProvisioningOutcome> {
  if (!allowExisting) {
    return { kind: "conflict", reason: "already_exists" };
  }
  let existing: InternalUserShape | null;
  try {
    existing = await repository.findUserByEmail(claimIdentity);
  } catch (error) {
    throw new TrustedIdentityProvisioningError(
      "dependency_failure",
      "failed to locate existing trusted identity",
      undefined,
      { cause: error },
    );
  }
  if (!existing) {
    return { kind: "conflict", reason: "already_exists" };
  }
  if (
    !existing.emailVerified ||
    normalizeCanonicalEmail(existing.email) !== claimIdentity ||
    normalizeAuthRole(existing.role) !== role
  ) {
    return { kind: "conflict", reason: "identity_mismatch" };
  }
  return { kind: "existing", user: existing, identity };
}
