import type { FastifyInstance, preHandlerHookHandler } from 'fastify'
import {
  extractAuthUser,
  extractErrorMessage,
  isDuplicateUserError,
  normalizeAuthRole,
  normalizeOptionalPositiveInteger,
  normalizeOptionalText,
  normalizeUsername,
} from './auth-utils.js'
import { getHeaderValue, sendError, sendSuccess, toHeaders } from './http-response.js'

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

export type InternalAuthAPI = {
  getSession(input: { headers: Headers }): Promise<unknown>
  signUpEmail(input: {
    body: {
      email: string
      password: string
      name: string
      username: string
    }
  }): Promise<unknown>
}

export type AuthUserRepository = {
  findUserByUsernameOrEmail(username: string, email: string): Promise<{
    id: string
    username?: string | null
    email?: string | null
    name?: string | null
    role?: string | null
  } | null>
  updateAuthUserRole(authUserID: string, role: 'admin' | 'user'): Promise<boolean>
  updateAuthUserBanState(
    authUserID: string,
    banned: boolean,
    banReason: string | null,
    banExpiresIn: number | null,
  ): Promise<boolean>
  deleteAuthUserSessions(authUserID: string): Promise<boolean>
  removeAuthUserCascade(authUserID: string): Promise<boolean>
}

export type InternalRoutesOptions = {
  internalKey: string
  allowInsecureInternalBypass: boolean
  authApi: InternalAuthAPI
  repository: AuthUserRepository
}

type AuthUserIDValidation = {
  value: string
  error: string
}

function validateAuthUserID(raw: unknown): AuthUserIDValidation {
  const value = String(raw ?? '').trim()
  if (!value) return { value: '', error: 'auth user id is required' }
  if (value.length > 191) return { value: '', error: 'auth user id is invalid' }
  return { value, error: '' }
}

export function registerInternalRoutes(app: FastifyInstance, options: InternalRoutesOptions) {
  const requireInternalKey = createRequireInternalKey(options)

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
      sendSuccess(reply, {
        session: {
          auth_user_id: user.id,
          username: normalizeUsername(user.username || user.name || ''),
          email: String(user.email || ''),
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
      const username = normalizeUsername(request.body?.username)
      const password = String(request.body?.password || '')
      const allowExisting = Boolean(request.body?.allow_existing)
      const role = normalizeAuthRole(request.body?.role)

      if (!username) {
        sendError(reply, 400, 'username is required')
        return
      }
      if (password.length < 6 || password.length > 50) {
        sendError(reply, 400, 'password length must be between 6 and 50')
        return
      }
      if (!role) {
        sendError(reply, 400, 'role must be admin or user')
        return
      }

      const email = `${username}@ideaboard.local`
      try {
        const created = await options.authApi.signUpEmail({
          body: {
            email,
            password,
            name: username,
            username,
          },
        })
        const user = extractAuthUser(created)
        if (!user) {
          sendError(reply, 500, 'auth user creation succeeded but user payload is empty')
          return
        }
        await options.repository.updateAuthUserRole(user.id, role)
        sendSuccess(reply, {
          user: {
            auth_user_id: user.id,
            username: normalizeUsername(user.username || user.name || username),
            email: String(user.email || email),
            role,
            existing: false,
          },
        })
      } catch (error) {
        if (allowExisting && isDuplicateUserError(error)) {
          try {
            const existing = await options.repository.findUserByUsernameOrEmail(username, email)
            if (existing) {
              await options.repository.updateAuthUserRole(existing.id, role)
              sendSuccess(reply, {
                user: {
                  auth_user_id: existing.id,
                  username: normalizeUsername(existing.username || existing.name || username),
                  email: String(existing.email || email),
                  role,
                  existing: true,
                },
              })
              return
            }
          } catch (lookupError) {
            sendError(reply, 500, extractErrorMessage(lookupError, 'failed to locate existing user'))
            return
          }
        }
        sendError(reply, 400, extractErrorMessage(error, 'failed to ensure user'))
      }
    },
  )

  app.put<{ Body: RoleBody; Params: RoleParams }>(
    '/internal/users/:id/role',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      const authUserID = validateAuthUserID(request.params?.id)
      const role = normalizeAuthRole(request.body?.role)
      if (authUserID.error) {
        sendError(reply, 400, authUserID.error)
        return
      }
      if (!role) {
        sendError(reply, 400, 'role must be admin or user')
        return
      }

      try {
        const updated = await options.repository.updateAuthUserRole(authUserID.value, role)
        if (!updated) {
          sendError(reply, 404, 'auth user not found')
          return
        }
        sendSuccess(reply, {
          auth_user_id: authUserID.value,
          role,
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to update auth user role'))
      }
    },
  )

  app.delete<{ Params: AuthUserIDParams }>(
    '/internal/users/:id',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      const authUserID = validateAuthUserID(request.params?.id)
      if (authUserID.error) {
        sendError(reply, 400, authUserID.error)
        return
      }

      try {
        const deleted = await options.repository.removeAuthUserCascade(authUserID.value)
        if (!deleted) {
          sendError(reply, 404, 'auth user not found')
          return
        }
        sendSuccess(reply, {
          deleted: true,
          auth_user_id: authUserID.value,
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to delete auth user'))
      }
    },
  )

  app.post<{ Body: BanUserBody }>(
    '/internal/users/ban',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      const authUserID = validateAuthUserID(request.body?.userId)
      const banReason = normalizeOptionalText(request.body?.banReason, 500)
      const banExpiresIn = normalizeOptionalPositiveInteger(request.body?.banExpiresIn)
      if (authUserID.error) {
        sendError(reply, 400, authUserID.error)
        return
      }
      try {
        const banned = await options.repository.updateAuthUserBanState(
          authUserID.value,
          true,
          banReason,
          banExpiresIn,
        )
        if (!banned) {
          sendError(reply, 404, 'auth user not found')
          return
        }
        await options.repository.deleteAuthUserSessions(authUserID.value)
        sendSuccess(reply, {
          auth_user_id: authUserID.value,
          banned: true,
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to ban auth user'))
      }
    },
  )

  app.post<{ Body: UserIDBody }>(
    '/internal/users/unban',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      const authUserID = validateAuthUserID(request.body?.userId)
      if (authUserID.error) {
        sendError(reply, 400, authUserID.error)
        return
      }
      try {
        const updated = await options.repository.updateAuthUserBanState(
          authUserID.value,
          false,
          null,
          null,
        )
        if (!updated) {
          sendError(reply, 404, 'auth user not found')
          return
        }
        sendSuccess(reply, {
          auth_user_id: authUserID.value,
          banned: false,
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to unban auth user'))
      }
    },
  )

  app.post<{ Body: UserIDBody }>(
    '/internal/users/revoke-sessions',
    { preHandler: requireInternalKey },
    async (request, reply) => {
      const authUserID = validateAuthUserID(request.body?.userId)
      if (authUserID.error) {
        sendError(reply, 400, authUserID.error)
        return
      }
      try {
        const deleted = await options.repository.deleteAuthUserSessions(authUserID.value)
        if (!deleted) {
          sendError(reply, 404, 'auth user not found')
          return
        }
        sendSuccess(reply, {
          auth_user_id: authUserID.value,
          revoked: true,
        })
      } catch (error) {
        sendError(reply, 500, extractErrorMessage(error, 'failed to revoke auth user sessions'))
      }
    },
  )
}

function createRequireInternalKey(options: InternalRoutesOptions): preHandlerHookHandler {
  return (request, reply, done) => {
    if (!options.internalKey) {
      if (options.allowInsecureInternalBypass) {
        done()
        return
      }
      sendError(reply, 500, 'internal key is not configured')
      done()
      return
    }
    const provided = getHeaderValue(request.headers['x-internal-key']).trim()
    if (!provided || provided !== options.internalKey) {
      sendError(reply, 401, 'unauthorized internal request')
      done()
      return
    }
    done()
  }
}
