import {
  extractAuthUser,
  isValidCanonicalEmail,
  normalizeCanonicalEmail,
  normalizeRegistrationUsername,
  type InternalUserShape,
} from "./authUtils.js";
import type { AuthRole } from "./generated/auth-internal/types.gen.js";

export type { AuthRole } from "./generated/auth-internal/types.gen.js";

export type TrustedIdentityAuthAPI = {
  signUpEmail(input: {
    body: {
      email: string;
      password: string;
      name: string;
      username: string;
    };
  }): Promise<unknown>;
};

export type VerifiedSessionIdentity = {
  auth_user_id: string;
  email: string;
  username: string;
  claim_identity: string;
  claim_identity_kind: "canonical_email" | "untrusted";
  claim_identity_verified: boolean;
};

export function sessionIdentityFromAuthUser(
  user: InternalUserShape,
): VerifiedSessionIdentity {
  const username = normalizeRegistrationUsername(user.username || user.name || "");
  const claimIdentity = normalizeCanonicalEmail(user.email);
  const isTrustedClaimIdentity =
    Boolean(user.emailVerified) && isValidCanonicalEmail(claimIdentity);

  return {
    auth_user_id: user.id.trim(),
    email: claimIdentity,
    username,
    claim_identity: claimIdentity,
    claim_identity_kind: isTrustedClaimIdentity ? "canonical_email" : "untrusted",
    claim_identity_verified: isTrustedClaimIdentity,
  };
}

export type TrustedIdentityRepository = {
  findUserByUsernameOrEmail(
    username: string,
    email: string,
  ): Promise<InternalUserShape | null>;
  finalizeTrustedRegistrationIdentity(
    authUserID: string,
    claimIdentity: string,
    role: AuthRole,
  ): Promise<boolean>;
  removeAuthUserCascade(authUserID: string): Promise<boolean>;
};

export class TrustedIdentityCleanupError extends Error {}

export async function provisionTrustedCanonicalIdentity(
  authApi: TrustedIdentityAuthAPI,
  repository: TrustedIdentityRepository,
  email: string,
  username: string,
  password: string,
  role: AuthRole,
) {
  const claimIdentity = normalizeCanonicalEmail(email);
  if (!isValidCanonicalEmail(claimIdentity))
    throw new Error("a valid email is required");
  let createdUserID = "";
  try {
    const created = await authApi.signUpEmail({
      body: { email: claimIdentity, password, name: username, username },
    });
    const user = extractAuthUser(created);
    if (!user) {
      let recovered;
      try {
        recovered = await repository.findUserByUsernameOrEmail(
          username,
          claimIdentity,
        );
      } catch (lookupError) {
        console.error(
          "[auth] failed to recover trusted identity after empty sign-up payload",
          lookupError,
        );
        throw new TrustedIdentityCleanupError(
          "trusted identity provisioning failed and cleanup requires operator attention",
        );
      }
      if (!recovered?.id) {
        throw new TrustedIdentityCleanupError(
          "trusted identity sign-up returned an empty payload and cleanup identity could not be recovered",
        );
      }
      createdUserID = recovered.id;
      throw new Error("auth user creation succeeded but user payload is empty");
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
    return user;
  } catch (error) {
    if (createdUserID) {
      try {
        const cleaned = await repository.removeAuthUserCascade(createdUserID);
        if (!cleaned) {
          throw new Error("created auth user was not found during cleanup");
        }
      } catch (cleanupError) {
        console.error(
          "[auth] failed to roll back incomplete trusted identity",
          cleanupError,
        );
        throw new TrustedIdentityCleanupError(
          "trusted identity provisioning failed and cleanup requires operator attention",
        );
      }
    }
    throw error;
  }
}
