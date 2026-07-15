import type { IncomingMessage, ServerResponse } from 'node:http'
import type { AuthNodeHandler } from './authEntryGuard.js'
import {
  provisionTrustedCanonicalIdentity,
  TrustedIdentityCleanupError,
  trustedCanonicalClaimIdentity,
  type TrustedIdentityAuthAPI,
  type TrustedIdentityRepository,
} from './trustedIdentity.js'
import {
  getAuthSubpath,
  isDuplicateUserError,
  isReservedRegistrationUsername,
  normalizeRegistrationUsername,
} from './authUtils.js'
import { sendNodeJSON } from './httpResponse.js'

const publicUsernameRegistrationPath = '/register/username'
const trustedClaimIdentityKind = 'trusted_canonical_registration'

export function createAuthDispatchHandler(
  betterAuthHandler: AuthNodeHandler,
  authApi: TrustedIdentityAuthAPI,
  repository: TrustedIdentityRepository,
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
  authApi: TrustedIdentityAuthAPI,
  repository: TrustedIdentityRepository,
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
    if (isDuplicateUserError(error)) {
      sendNodeJSON(res, 409, { code: 409, msg: 'username already exists' })
      return
    }
    console.error('[auth] trusted username registration failed', error)
    const cleanupRequired = error instanceof TrustedIdentityCleanupError
    const message = cleanupRequired
      ? 'trusted identity provisioning failed and cleanup requires operator attention'
      : 'registration failed'
    sendNodeJSON(res, 500, {
      code: 500,
      msg: message,
    })
  }
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
