import type { IncomingMessage, ServerResponse } from 'node:http'
import type { AuthNodeHandler } from './authEntryGuard.js'
import type { AuthUserRepository, InternalAuthAPI } from './internalRoutes.js'
import {
  extractAuthUser,
  extractErrorMessage,
  getAuthSubpath,
  isReservedRegistrationUsername,
  normalizeRegistrationUsername,
} from './authUtils.js'
import { sendNodeJSON } from './httpResponse.js'

const publicUsernameRegistrationPath = '/register/username'
const trustedClaimIdentityKind = 'trusted_canonical_registration'

export class TrustedIdentityCleanupError extends Error {}

export function createAuthDispatchHandler(
  betterAuthHandler: AuthNodeHandler,
  authApi: InternalAuthAPI,
  repository: AuthUserRepository,
): AuthNodeHandler {
  return async (req, res) => {
    const path = getAuthSubpath(req.url || '')
    if (path === publicUsernameRegistrationPath) {
      await registerUsername(req, res, authApi, repository)
      return
    }
    if (
      path === '/sign-up' ||
      path === '/sign-up/email' ||
      path.startsWith('/sign-up/')
    ) {
      sendNodeJSON(res, 404, { code: 404, msg: '仅支持 username 注册' })
      return
    }
    await betterAuthHandler(req, res)
  }
}

async function registerUsername(
  req: IncomingMessage,
  res: ServerResponse,
  authApi: InternalAuthAPI,
  repository: AuthUserRepository,
) {
  if (req.method !== 'POST') {
    sendNodeJSON(res, 405, { code: 405, msg: 'method not allowed' })
    return
  }

  let body: { username?: unknown; password?: unknown }
  try {
    body = JSON.parse(await readRequestBody(req)) as {
      username?: unknown
      password?: unknown
    }
  } catch {
    sendNodeJSON(res, 400, { code: 400, msg: 'invalid JSON body' })
    return
  }
  const username = normalizeRegistrationUsername(body.username)
  const password = String(body.password || '')
  if (username.length < 3) {
    sendNodeJSON(res, 400, {
      code: 400,
      msg: 'username length must be between 3 and 50',
    })
    return
  }
  if (isReservedRegistrationUsername(username)) {
    sendNodeJSON(res, 409, { code: 409, msg: 'username is reserved' })
    return
  }
  if (password.length < 6 || password.length > 50) {
    sendNodeJSON(res, 400, {
      code: 400,
      msg: 'password length must be between 6 and 50',
    })
    return
  }

  const claimIdentity = trustedCanonicalClaimIdentity(username)
  try {
    const user = await provisionTrustedCanonicalIdentity(
      authApi,
      repository,
      username,
      password,
      'user',
    )
    sendNodeJSON(res, 200, {
      code: 0,
      msg: 'success',
      data: {
        user: {
          auth_user_id: user.id,
          username,
          claim_identity: claimIdentity,
          claim_identity_kind: trustedClaimIdentityKind,
          claim_identity_verified: true,
        },
      },
    })
  } catch (error) {
    const status = error instanceof TrustedIdentityCleanupError ? 500 : 400
    sendNodeJSON(res, status, {
      code: status,
      msg: extractErrorMessage(error, 'registration failed'),
    })
  }
}

export async function provisionTrustedCanonicalIdentity(
  authApi: InternalAuthAPI,
  repository: AuthUserRepository,
  username: string,
  password: string,
  role: 'admin' | 'user',
) {
  const claimIdentity = trustedCanonicalClaimIdentity(username)
  let createdUserID = ''
  try {
    const created = await authApi.signUpEmail({
      body: { email: claimIdentity, password, name: username, username },
    })
    const user = extractAuthUser(created)
    if (!user) {
      let recovered
      try {
        recovered = await repository.findUserByUsernameOrEmail(username, claimIdentity)
      } catch (lookupError) {
        throw new TrustedIdentityCleanupError(
          `trusted identity lookup failed after empty sign-up payload: ${extractErrorMessage(lookupError, 'unknown lookup error')}`,
        )
      }
      if (!recovered?.id) {
        throw new TrustedIdentityCleanupError(
          'trusted identity sign-up returned an empty payload and cleanup identity could not be recovered',
        )
      }
      createdUserID = recovered.id
      throw new Error('auth user creation succeeded but user payload is empty')
    }
    createdUserID = user.id
    if (!(await repository.finalizeTrustedRegistrationIdentity(user.id, claimIdentity, role))) {
      throw new Error('failed to finalize trusted canonical registration identity')
    }
    return user
  } catch (error) {
    if (createdUserID) {
      try {
        const cleaned = await repository.removeAuthUserCascade(createdUserID)
        if (!cleaned) {
          throw new Error('created auth user was not found during cleanup')
        }
      } catch (cleanupError) {
        console.error('[auth] failed to roll back incomplete trusted identity', cleanupError)
        throw new TrustedIdentityCleanupError(
          'trusted identity provisioning failed and cleanup requires operator attention',
        )
      }
    }
    throw error
  }
}

export function trustedCanonicalClaimIdentity(username: string): string {
  return `${normalizeRegistrationUsername(username)}@users.ideaboard.local`
}

async function readRequestBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const chunk of req) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
    size += buffer.length
    if (size > 16_384) {
      throw new Error('request body is too large')
    }
    chunks.push(buffer)
  }
  return Buffer.concat(chunks).toString('utf8')
}
