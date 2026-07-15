import type { InternalUserShape } from './authUtils.js'
import {
  isDuplicateUserError,
  normalizeAuthRole,
  normalizeOptionalPositiveInteger,
  normalizeOptionalText,
  normalizeRegistrationUsername,
  normalizeUsername,
} from './authUtils.js'
import {
  provisionTrustedCanonicalIdentity,
  trustedCanonicalClaimIdentity,
  type AuthRole,
  type TrustedIdentityAuthAPI,
  type TrustedIdentityRepository,
} from './trustedIdentity.js'

export type { AuthRole, TrustedIdentityAuthAPI, TrustedIdentityRepository } from './trustedIdentity.js'

export type InternalAuthAPI = TrustedIdentityAuthAPI & {
  getSession(input: { headers: Headers }): Promise<unknown>
}

export type AuthInternalCommandRepository = TrustedIdentityRepository & {
  updateAuthUserRole(authUserID: string, role: AuthRole): Promise<boolean>
  updateAuthUserBanState(
    authUserID: string,
    banned: boolean,
    banReason: string | null,
    banExpiresIn: number | null,
  ): Promise<boolean>
  deleteAuthUserSessions(authUserID: string): Promise<boolean>
}

export type AuthInternalCommandFailureKind =
  | 'invalid_input'
  | 'not_found'
  | 'conflict'
  | 'repository_failure'

export type AuthInternalCommandResult<TData> =
  | {
      ok: true
      data: TData
    }
  | {
      ok: false
      kind: AuthInternalCommandFailureKind
      message: string
    }

export type EnsureIdentityCommandInput = {
  username?: unknown
  password?: unknown
  allowExisting?: unknown
  role?: unknown
}

export type AuthUserIDCommandInput = {
  authUserID?: unknown
}

export type BanIdentityCommandInput = AuthUserIDCommandInput & {
  banReason?: unknown
  banExpiresIn?: unknown
}

export type SetRoleCommandInput = AuthUserIDCommandInput & {
  role?: unknown
}

export type TrustedIdentityData = {
  user: {
    auth_user_id: string
    username: string
    email: string
    claim_identity: string
    claim_identity_kind: 'trusted_canonical_registration'
    claim_identity_verified: true
    role: AuthRole
    existing: boolean
  }
}

export type AuthInternalCommands = {
  ensureIdentity(input: EnsureIdentityCommandInput): Promise<AuthInternalCommandResult<TrustedIdentityData>>
  deleteIdentity(input: AuthUserIDCommandInput): Promise<AuthInternalCommandResult<{
    deleted: true
    auth_user_id: string
  }>>
  banIdentity(input: BanIdentityCommandInput): Promise<AuthInternalCommandResult<{
    auth_user_id: string
    banned: true
  }>>
  unbanIdentity(input: AuthUserIDCommandInput): Promise<AuthInternalCommandResult<{
    auth_user_id: string
    banned: false
  }>>
  revokeSessions(input: AuthUserIDCommandInput): Promise<AuthInternalCommandResult<{
    auth_user_id: string
    revoked: true
  }>>
  setRole(input: SetRoleCommandInput): Promise<AuthInternalCommandResult<{
    auth_user_id: string
    role: AuthRole
  }>>
}

export function createAuthInternalCommands(
  authApi: TrustedIdentityAuthAPI,
  repository: AuthInternalCommandRepository,
): AuthInternalCommands {
  return {
    ensureIdentity: (input) => ensureIdentity(input, authApi, repository),
    deleteIdentity: (input) => deleteIdentity(input, repository),
    banIdentity: (input) => banIdentity(input, repository),
    unbanIdentity: (input) => unbanIdentity(input, repository),
    revokeSessions: (input) => revokeSessions(input, repository),
    setRole: (input) => setRole(input, repository),
  }
}

async function ensureIdentity(
  input: EnsureIdentityCommandInput,
  authApi: TrustedIdentityAuthAPI,
  repository: AuthInternalCommandRepository,
): Promise<AuthInternalCommandResult<TrustedIdentityData>> {
  const username = normalizeRegistrationUsername(input.username)
  const password = String(input.password || '')
  const allowExisting = input.allowExisting === true
  const role = normalizeAuthRole(input.role)

  if (!username) return commandFailure('invalid_input', 'username is required')
  if (password.length < 6 || password.length > 50) {
    return commandFailure('invalid_input', 'password length must be between 6 and 50')
  }
  if (!role) return commandFailure('invalid_input', 'role must be admin or user')

  const claimIdentity = trustedCanonicalClaimIdentity(username)
  try {
    const user = await provisionTrustedCanonicalIdentity(
      authApi,
      repository,
      username,
      password,
      role,
    )
    return commandSuccess(trustedIdentityData(user, username, claimIdentity, role, false))
  } catch (error) {
    if (!isDuplicateUserError(error)) {
      return repositoryFailure(error, 'failed to ensure user')
    }
    if (!allowExisting) {
      return commandFailure('conflict', 'auth identity already exists')
    }
  }

  try {
    const existing = await repository.findUserByUsernameOrEmail(username, claimIdentity)
    if (!existing) {
      return commandFailure('conflict', 'auth identity already exists')
    }
    const existingUsername = normalizeRegistrationUsername(existing.username || existing.name || '')
    const existingEmail = String(existing.email || '').toLowerCase()
    if (
      !existing.emailVerified ||
      existingUsername !== username ||
      existingEmail !== claimIdentity ||
      existing.role !== role
    ) {
      return commandFailure('conflict', 'existing identity conflicts with requested trusted user')
    }
    return commandSuccess(
      trustedIdentityData(existing, username, claimIdentity, role, true),
    )
  } catch (error) {
    return repositoryFailure(error, 'failed to locate existing user')
  }
}

async function deleteIdentity(
  input: AuthUserIDCommandInput,
  repository: AuthInternalCommandRepository,
) {
  return runAuthUserMutation(
    input,
    (authUserID) => repository.removeAuthUserCascade(authUserID),
    'failed to delete auth user',
    (authUserID) => ({
      deleted: true as const,
      auth_user_id: authUserID,
    }),
  )
}

async function banIdentity(
  input: BanIdentityCommandInput,
  repository: AuthInternalCommandRepository,
): Promise<AuthInternalCommandResult<{ auth_user_id: string; banned: true }>> {
  const authUserID = validateAuthUserID(input.authUserID)
  if (!authUserID.ok) return authUserID.result

  try {
    const banned = await repository.updateAuthUserBanState(
      authUserID.value,
      true,
      normalizeOptionalText(input.banReason, 500),
      normalizeOptionalPositiveInteger(input.banExpiresIn),
    )
    if (!banned) return commandFailure('not_found', 'auth user not found')

    const revoked = await repository.deleteAuthUserSessions(authUserID.value)
    if (!revoked) return commandFailure('not_found', 'auth user not found')

    return commandSuccess({ auth_user_id: authUserID.value, banned: true })
  } catch (error) {
    return repositoryFailure(error, 'failed to ban auth user')
  }
}

async function unbanIdentity(
  input: AuthUserIDCommandInput,
  repository: AuthInternalCommandRepository,
) {
  return runAuthUserMutation(
    input,
    (authUserID) => repository.updateAuthUserBanState(authUserID, false, null, null),
    'failed to unban auth user',
    (authUserID) => ({ auth_user_id: authUserID, banned: false as const }),
  )
}

async function revokeSessions(
  input: AuthUserIDCommandInput,
  repository: AuthInternalCommandRepository,
) {
  return runAuthUserMutation(
    input,
    (authUserID) => repository.deleteAuthUserSessions(authUserID),
    'failed to revoke auth user sessions',
    (authUserID) => ({ auth_user_id: authUserID, revoked: true as const }),
  )
}

async function setRole(
  input: SetRoleCommandInput,
  repository: AuthInternalCommandRepository,
): Promise<AuthInternalCommandResult<{ auth_user_id: string; role: AuthRole }>> {
  const authUserID = validateAuthUserID(input.authUserID)
  if (!authUserID.ok) return authUserID.result

  const role = normalizeAuthRole(input.role)
  if (!role) return commandFailure('invalid_input', 'role must be admin or user')

  try {
    const updated = await repository.updateAuthUserRole(authUserID.value, role)
    if (!updated) return commandFailure('not_found', 'auth user not found')
    return commandSuccess({ auth_user_id: authUserID.value, role })
  } catch (error) {
    return repositoryFailure(error, 'failed to update auth user role')
  }
}

async function runAuthUserMutation<TData>(
  input: AuthUserIDCommandInput,
  mutate: (authUserID: string) => Promise<boolean>,
  failureMessage: string,
  data: (authUserID: string) => TData,
): Promise<AuthInternalCommandResult<TData>> {
  const authUserID = validateAuthUserID(input.authUserID)
  if (!authUserID.ok) return authUserID.result

  try {
    const updated = await mutate(authUserID.value)
    if (!updated) return commandFailure('not_found', 'auth user not found')
    return commandSuccess(data(authUserID.value))
  } catch (error) {
    return repositoryFailure(error, failureMessage)
  }
}

function trustedIdentityData(
  user: InternalUserShape,
  username: string,
  claimIdentity: string,
  role: AuthRole,
  existing: boolean,
): TrustedIdentityData {
  return {
    user: {
      auth_user_id: user.id,
      username: normalizeUsername(user.username || user.name || username),
      email: String(user.email || claimIdentity),
      claim_identity: claimIdentity,
      claim_identity_kind: 'trusted_canonical_registration',
      claim_identity_verified: true,
      role,
      existing,
    },
  }
}

type AuthUserIDValidation =
  | { ok: true; value: string }
  | { ok: false; result: AuthInternalCommandResult<never> }

function validateAuthUserID(raw: unknown): AuthUserIDValidation {
  const value = String(raw ?? '').trim()
  if (!value) {
    return { ok: false, result: commandFailure('invalid_input', 'auth user id is required') }
  }
  if (value.length > 191) {
    return { ok: false, result: commandFailure('invalid_input', 'auth user id is invalid') }
  }
  return { ok: true, value }
}

function commandSuccess<TData>(data: TData): AuthInternalCommandResult<TData> {
  return { ok: true, data }
}

function commandFailure(
  kind: AuthInternalCommandFailureKind,
  message: string,
): AuthInternalCommandResult<never> {
  return { ok: false, kind, message }
}

function repositoryFailure(error: unknown, fallback: string): AuthInternalCommandResult<never> {
  console.error(`[auth] ${fallback}`, error)
  return commandFailure('repository_failure', fallback)
}
