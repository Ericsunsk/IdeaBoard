import type {
  FastifyInstance,
  FastifyReply,
  preHandlerHookHandler,
} from 'fastify'
import type {
  AuthInternalCommandFailureKind,
  AuthInternalCommandResult,
  AuthInternalCommands,
  InternalAuthAPI,
} from './authInternalCommands.js'
import {
  extractAuthUser,
  extractErrorMessage,
  normalizeRegistrationUsername,
} from './authUtils.js'
import { getHeaderValue, sendError, sendSuccess, toHeaders } from './httpResponse.js'
import { trustedCanonicalClaimIdentity } from './trustedIdentity.js'

type EnsureUserBody = {
  username?: unknown
  password?: unknown
  allow_existing?: unknown
  role?: unknown
}

type RoleParams = {
  id?: string
}

type RoleBody = {
  role?: unknown
}

type AuthUserIDParams = {
  id?: string
}

type UserIDBody = {
  userId?: unknown
}

type BanUserBody = UserIDBody & {
  banReason?: unknown
  banExpiresIn?: unknown
}

export type InternalRoutesOptions = {
  internalKey: string
  authApi: Pick<InternalAuthAPI, 'getSession'>
  commands: AuthInternalCommands
}

const commandFailureStatuses = {
  invalid_input: 400,
  not_found: 404,
  conflict: 409,
  repository_failure: 500,
} satisfies Record<AuthInternalCommandFailureKind, number>

export function registerInternalRoutes(app: FastifyInstance, options: InternalRoutesOptions) {
  const requireInternalKey = createRequireInternalKey(options.internalKey)

  app.get('/internal/session', { preHandler: requireInternalKey }, async (request, reply) => {
    try {
      const session = await options.authApi.getSession({
        headers: toHeaders(request.headers),
      })
      const user = extractAuthUser(session)
      if (!user) {
        sendError(reply, 401, 'unauthorized')
        return
      }
      const username = normalizeRegistrationUsername(user.username || user.name || '')
      const claimIdentity = String(user.email || '').toLowerCase()
      const isTrustedClaimIdentity =
        Boolean(user.emailVerified) && claimIdentity === trustedCanonicalClaimIdentity(username)
      sendSuccess(reply, {
        session: {
          auth_user_id: user.id,
          username,
          claim_identity: claimIdentity,
          claim_identity_kind: isTrustedClaimIdentity
            ? 'trusted_canonical_registration'
            : 'untrusted',
          claim_identity_verified: isTrustedClaimIdentity,
        },
      })
    } catch (error) {
      sendError(reply, 500, extractErrorMessage(error, 'failed to validate session'))
    }
  })

  app.post<{ Body: EnsureUserBody }>(
    '/internal/users/ensure',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.ensureIdentity({
          username: request.body?.username,
          password: request.body?.password,
          allowExisting: request.body?.allow_existing,
          role: request.body?.role,
        }),
      )
    },
  )

  app.put<{ Body: RoleBody; Params: RoleParams }>(
    '/internal/users/:id/role',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.setRole({
          authUserID: request.params?.id,
          role: request.body?.role,
        }),
      )
    },
  )

  app.delete<{ Params: AuthUserIDParams }>(
    '/internal/users/:id',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.deleteIdentity({ authUserID: request.params?.id }),
      )
    },
  )

  app.post<{ Body: BanUserBody }>(
    '/internal/users/ban',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.banIdentity({
          authUserID: request.body?.userId,
          banReason: request.body?.banReason,
          banExpiresIn: request.body?.banExpiresIn,
        }),
      )
    },
  )

  app.post<{ Body: UserIDBody }>(
    '/internal/users/unban',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.unbanIdentity({ authUserID: request.body?.userId }),
      )
    },
  )

  app.post<{ Body: UserIDBody }>(
    '/internal/users/revoke-sessions',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.revokeSessions({ authUserID: request.body?.userId }),
      )
    },
  )
}

function sendCommandResult<TData>(
  reply: FastifyReply,
  result: AuthInternalCommandResult<TData>,
) {
  if (!result.ok) {
    sendError(reply, commandFailureStatuses[result.kind], result.message)
    return
  }
  sendSuccess(reply, result.data)
}

function createRequireInternalKey(internalKey: string): preHandlerHookHandler {
  return (request, reply, done) => {
    if (!internalKey) {
      sendError(reply, 500, 'internal key is not configured')
      done()
      return
    }
    const provided = getHeaderValue(request.headers['x-internal-key']).trim()
    if (!provided || provided !== internalKey) {
      sendError(reply, 401, 'unauthorized internal request')
      done()
      return
    }
    done()
  }
}
