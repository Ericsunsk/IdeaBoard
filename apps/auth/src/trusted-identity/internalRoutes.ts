import type { FastifyInstance, FastifyReply, preHandlerHookHandler } from 'fastify'
import type {
  AuthInternalCommandFailureKind,
  AuthInternalCommandResult,
  AuthInternalCommands,
  AuthSessionAPI,
} from './authInternalCommands.js'
import { extractAuthUser, extractErrorMessage } from '../infrastructure/authUtils.js'
import type { RouteHandlers } from '../generated/auth-internal/fastify.gen.js'
import { authInternalRoutes } from '../generated/auth-internal/routes.gen.js'
import {
  getHeaderValue,
  sendError,
  sendSuccess,
  toHeaders,
} from '../infrastructure/httpResponse.js'
import { sessionIdentityFromAuthUser } from './trustedIdentity.js'

export type InternalRoutesOptions = {
  internalKey: string
  authApi: AuthSessionAPI
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
  const handlers: Pick<
    RouteHandlers,
    | 'validateSession'
    | 'ensureUser'
    | 'getUserEmails'
    | 'getUserStates'
    | 'deleteUser'
    | 'setUserRole'
    | 'banUser'
    | 'unbanUser'
    | 'revokeUserSessions'
  > = {
    validateSession: async (request, reply) => {
      try {
        const session = await options.authApi.getSession({
          headers: toHeaders(request.headers),
        })
        const user = extractAuthUser(session)
        if (!user) {
          sendError(reply, 401, 'unauthorized')
          return
        }
        sendSuccess(reply, {
          session: sessionIdentityFromAuthUser(user),
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to validate session'))
      }
    },
    ensureUser: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.ensureIdentity({
          email: request.body?.email,
          displayName: request.body?.display_name,
          password: request.body?.password,
          allowExisting: request.body?.allow_existing,
          role: request.body?.role,
        }),
      )
    },
    getUserEmails: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.getUserEmails({
          userIDs: request.body?.user_ids,
        }),
      )
    },
    getUserStates: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.getUserStates({
          userIDs: request.body?.user_ids,
        }),
      )
    },
    deleteUser: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.deleteIdentity({
          authUserID: request.params?.id,
        }),
      )
    },
    setUserRole: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.setRole({
          authUserID: request.params?.id,
          role: request.body?.role,
        }),
      )
    },
    banUser: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.banIdentity({
          authUserID: request.body?.userId,
          banExpiresIn: request.body?.banExpiresIn,
        }),
      )
    },
    unbanUser: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.unbanIdentity({
          authUserID: request.body?.userId,
        }),
      )
    },
    revokeUserSessions: async (request, reply) => {
      sendCommandResult(
        reply,
        await options.commands.revokeSessions({
          authUserID: request.body?.userId,
        }),
      )
    },
  }

  app.route({
    method: authInternalRoutes.validateSession.method,
    url: authInternalRoutes.validateSession.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.validateSession,
  })
  app.route({
    method: authInternalRoutes.ensureUser.method,
    url: authInternalRoutes.ensureUser.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.ensureUser,
  })
  app.route({
    method: authInternalRoutes.getUserEmails.method,
    url: authInternalRoutes.getUserEmails.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.getUserEmails,
  })
  app.route({
    method: authInternalRoutes.getUserStates.method,
    url: authInternalRoutes.getUserStates.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.getUserStates,
  })
  app.route({
    method: authInternalRoutes.deleteUser.method,
    url: authInternalRoutes.deleteUser.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.deleteUser,
  })
  app.route({
    method: authInternalRoutes.setUserRole.method,
    url: authInternalRoutes.setUserRole.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.setUserRole,
  })
  app.route({
    method: authInternalRoutes.banUser.method,
    url: authInternalRoutes.banUser.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.banUser,
  })
  app.route({
    method: authInternalRoutes.unbanUser.method,
    url: authInternalRoutes.unbanUser.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.unbanUser,
  })
  app.route({
    method: authInternalRoutes.revokeUserSessions.method,
    url: authInternalRoutes.revokeUserSessions.fastifyPath,
    preHandler: requireInternalKey,
    handler: handlers.revokeUserSessions,
  })
}

function sendCommandResult<TData>(reply: FastifyReply, result: AuthInternalCommandResult<TData>) {
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
