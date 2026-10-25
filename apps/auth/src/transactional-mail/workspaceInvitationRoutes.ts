import type { FastifyInstance } from 'fastify'
import { createInternalKeyPreHandler } from '../admission/internalKeyGuard.js'
import { authInternalRoutes } from '../generated/auth-internal/routes.gen.js'
import type { WorkspaceInvitationMailRequestWritable } from '../generated/auth-internal/types.gen.js'
import { sendError, sendSuccess } from '../infrastructure/httpResponse.js'
import { MailConfigError, type MailConfigService } from './mailConfig.js'

const invitationFields = new Set([
  'recipient',
  'invite_id',
  'workspace_name',
  'inviter_name',
  'role',
  'token',
])

export function registerWorkspaceInvitationRoutes(
  app: FastifyInstance,
  options: {
    internalKey: string
    service: Pick<MailConfigService, 'sendWorkspaceInvitation'>
  },
): void {
  app.post(
    authInternalRoutes.sendWorkspaceInvitation.fastifyPath,
    { preHandler: createInternalKeyPreHandler(options.internalKey) },
    async (request, reply) => {
      reply.header('Cache-Control', 'no-store')
      if (!isInvitationRequest(request.body)) {
        sendError(reply, 400, 'invalid workspace invitation payload')
        return
      }
      const body = request.body
      try {
        await options.service.sendWorkspaceInvitation({
          recipient: body.recipient,
          inviteId: body.invite_id,
          workspaceName: body.workspace_name,
          inviterName: body.inviter_name,
          role: body.role,
          token: body.token,
        })
        sendSuccess(reply, { ok: true })
      } catch (error) {
        const status =
          error instanceof MailConfigError
            ? error.code === 'invalid_input'
              ? 400
              : error.code === 'provider_error'
                ? 502
                : 503
            : 500
        sendError(reply, status, 'workspace invitation delivery failed')
      }
    },
  )
}

function isInvitationRequest(value: unknown): value is WorkspaceInvitationMailRequestWritable {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const body = value as Record<string, unknown>
  if (Object.keys(body).some((key) => !invitationFields.has(key))) return false
  for (const key of invitationFields) {
    if (typeof body[key] !== 'string') return false
  }
  return body.role === 'admin' || body.role === 'member'
}
