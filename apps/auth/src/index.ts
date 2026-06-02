import Fastify, {
  type FastifyReply,
  type preHandlerHookHandler,
} from 'fastify'
import { toNodeHandler } from 'better-auth/node'
import type { IncomingHttpHeaders, IncomingMessage, ServerResponse } from 'node:http'
import { auth, authDB } from './auth.js'
import { env } from './env.js'
import { isProductionRuntime } from './runtime-env.js'

type InternalUserShape = {
  id: string
  username?: string | null
  email?: string | null
  name?: string | null
  role?: string | null
}

type MemoryRateLimitEntry = {
  count: number
  resetAtMs: number
}

type AuthGuardSettings = {
  registrationEnabled: boolean
  windowSeconds: number
  maxRequests: number
}

type AuthGuardSettingsCache = {
  value: AuthGuardSettings
  expiresAtMs: number
}

type AuthMiddlewareRequest = IncomingMessage & {
  socket: IncomingMessage['socket']
}

type InternalHeaders = IncomingHttpHeaders

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

// Trust only the configured reverse proxy ranges so request.ip reflects the real
// client IP from X-Forwarded-For without accepting arbitrary public proxies.
const app = Fastify({
  bodyLimit: 1_048_576,
  trustProxy: env.auth.trustProxy,
})
const betterAuthNodeHandler = toNodeHandler(auth)
const authRateLimitStore = new Map<string, MemoryRateLimitEntry>()
let authRateLimitCheckCounter = 0
const AUTH_GUARD_SETTINGS_TTL_MS = 10_000
const DEFAULT_AUTH_GUARD_SETTINGS: AuthGuardSettings = {
  registrationEnabled: false,
  windowSeconds: 60,
  maxRequests: 30,
}
const allowedAuthOrigins = new Set(env.auth.trustedOrigins.map(normalizeCORSOrigin).filter(Boolean))
let authGuardSettingsCache: AuthGuardSettingsCache | null = null

app.setErrorHandler((error, _request, reply) => {
  console.error('[auth] Unhandled request error', error)
  sendError(reply, getErrorStatus(error), extractErrorMessage(error, 'internal server error'))
})

app.addHook('onRequest', async (request, reply) => {
  if (!isAuthRoute(request.raw.url || '')) {
    return
  }

  const authReq = request.raw as AuthMiddlewareRequest

  const corsHandled = authCORSMiddleware(authReq, reply.raw)
  if (corsHandled) {
    reply.hijack()
    return
  }

  const guardHandled = await authEntryGuardMiddleware(authReq, reply.raw, request.ip)
  if (guardHandled) {
    reply.hijack()
    return
  }

  reply.hijack()
  await betterAuthNodeHandler(request.raw, reply.raw)
})

app.get('/api/health', async (_request, reply) => {
  sendSuccess(reply, { status: 'ok' })
})

const requireInternalKey: preHandlerHookHandler = (request, reply, done) => {
  if (!env.auth.internalKey) {
    done()
    return
  }
  const provided = getHeaderValue(request.headers['x-internal-key']).trim()
  if (!provided || provided !== env.auth.internalKey) {
    sendError(reply, 401, 'unauthorized internal request')
    done()
    return
  }
  done()
}

app.get('/internal/session', { preHandler: requireInternalKey }, async (request, reply) => {
  try {
    const session = await auth.api.getSession({
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
      const created = await auth.api.signUpEmail({
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
      await syncAuthUserRole(user.id, role)
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
          const existing = await findUserByUsernameOrEmail(username, email)
          if (existing) {
            await syncAuthUserRole(existing.id, role)
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
    const authUserID = String(request.params?.id || '').trim()
    const role = normalizeAuthRole(request.body?.role)
    if (!authUserID) {
      sendError(reply, 400, 'auth user id is required')
      return
    }
    if (authUserID.length > 191) {
      sendError(reply, 400, 'auth user id is invalid')
      return
    }
    if (!role) {
      sendError(reply, 400, 'role must be admin or user')
      return
    }

    try {
      const updated = await updateAuthUserRole(authUserID, role)
      if (!updated) {
        sendError(reply, 404, 'auth user not found')
        return
      }
      sendSuccess(reply, {
        auth_user_id: authUserID,
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
    const authUserID = String(request.params?.id || '').trim()
    if (!authUserID) {
      sendError(reply, 400, 'auth user id is required')
      return
    }
    if (authUserID.length > 191) {
      sendError(reply, 400, 'auth user id is invalid')
      return
    }

    try {
      const deleted = await removeAuthUserCascade(authUserID)
      if (!deleted) {
        sendError(reply, 404, 'auth user not found')
        return
      }
      sendSuccess(reply, {
        deleted: true,
        auth_user_id: authUserID,
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
    const authUserID = String(request.body?.userId || '').trim()
    const banReason = normalizeOptionalText(request.body?.banReason, 500)
    const banExpiresIn = normalizeOptionalPositiveInteger(request.body?.banExpiresIn)
    if (!authUserID) {
      sendError(reply, 400, 'auth user id is required')
      return
    }
    if (authUserID.length > 191) {
      sendError(reply, 400, 'auth user id is invalid')
      return
    }
    try {
      const banned = await updateAuthUserBanState(authUserID, true, banReason, banExpiresIn)
      if (!banned) {
        sendError(reply, 404, 'auth user not found')
        return
      }
      await deleteAuthUserSessions(authUserID)
      sendSuccess(reply, {
        auth_user_id: authUserID,
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
    const authUserID = String(request.body?.userId || '').trim()
    if (!authUserID) {
      sendError(reply, 400, 'auth user id is required')
      return
    }
    if (authUserID.length > 191) {
      sendError(reply, 400, 'auth user id is invalid')
      return
    }
    try {
      const updated = await updateAuthUserBanState(authUserID, false, null, null)
      if (!updated) {
        sendError(reply, 404, 'auth user not found')
        return
      }
      sendSuccess(reply, {
        auth_user_id: authUserID,
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
    const authUserID = String(request.body?.userId || '').trim()
    if (!authUserID) {
      sendError(reply, 400, 'auth user id is required')
      return
    }
    if (authUserID.length > 191) {
      sendError(reply, 400, 'auth user id is invalid')
      return
    }
    try {
      const deleted = await deleteAuthUserSessions(authUserID)
      if (!deleted) {
        sendError(reply, 404, 'auth user not found')
        return
      }
      sendSuccess(reply, {
        auth_user_id: authUserID,
        revoked: true,
      })
    } catch (error) {
      sendError(reply, 500, extractErrorMessage(error, 'failed to revoke auth user sessions'))
    }
  },
)

if (isProductionRuntime() && !env.auth.internalKey) {
  throw new Error('AUTH_INTERNAL_KEY is required when NODE_ENV=production')
}

try {
  await app.listen({ port: env.auth.port, host: env.auth.host })
  if (!env.auth.internalKey) {
    console.warn('[auth] AUTH_INTERNAL_KEY is empty; internal endpoints are not protected')
  }
  console.log(`[auth] Better Auth service listening on http://${env.auth.host}:${env.auth.port}`)
} catch (error) {
  console.error('[auth] Failed to start auth service', error)
  await authDB.end()
  process.exit(1)
}

function gracefulShutdown(signal: string) {
  console.log(`[auth] ${signal} received, shutting down gracefully...`)
  // Force exit after 10s if graceful close hangs
  setTimeout(() => {
    console.error('[auth] Graceful shutdown timed out, forcing exit')
    process.exit(1)
  }, 10_000).unref()

  app
    .close()
    .then(() => authDB.end())
    .then(() => {
      console.log('[auth] Database pool closed')
      process.exit(0)
    })
    .catch((err) => {
      console.error('[auth] Error closing auth service', err)
      process.exit(1)
    })
}

process.on('SIGTERM', () => gracefulShutdown('SIGTERM'))
process.on('SIGINT', () => gracefulShutdown('SIGINT'))

function isAuthRoute(url: string): boolean {
  const path = normalizeURLPath(url)
  return path === '/api/auth' || path.startsWith('/api/auth/')
}

function normalizeURLPath(rawURL: string): string {
  const rawPath = String(rawURL || '').split('?')[0] || '/'
  return normalizeAuthPath(rawPath)
}

function authCORSMiddleware(req: IncomingMessage, res: ServerResponse): boolean {
  const origin = getHeaderValue(req.headers.origin).trim()
  if (!origin) {
    return false
  }

  const isAllowed = allowedAuthOrigins.has(normalizeCORSOrigin(origin))
  if (!isAllowed) {
    if (req.method === 'OPTIONS') {
      sendNodeStatus(res, 403)
      return true
    }
    return false
  }

  res.setHeader('Access-Control-Allow-Origin', origin)
  res.setHeader('Access-Control-Allow-Credentials', 'true')
  res.setHeader('Access-Control-Allow-Methods', 'GET,POST,PUT,PATCH,DELETE,OPTIONS')
  res.setHeader(
    'Access-Control-Allow-Headers',
    getHeaderValue(req.headers['access-control-request-headers']) ||
      'Content-Type, Authorization, Cookie, X-Requested-With',
  )
  appendVaryHeader(res, 'Origin')

  if (req.method === 'OPTIONS') {
    sendNodeStatus(res, 204)
    return true
  }
  return false
}

function normalizeCORSOrigin(raw: string): string {
  try {
    const parsed = new URL(raw.trim())
    const protocol = parsed.protocol.toLowerCase()
    if (protocol !== 'http:' && protocol !== 'https:') {
      return ''
    }
    return `${protocol}//${parsed.host.toLowerCase()}`
  } catch {
    return ''
  }
}

async function getAuthGuardSettings(): Promise<AuthGuardSettings> {
  const nowMs = Date.now()
  if (authGuardSettingsCache && nowMs < authGuardSettingsCache.expiresAtMs) {
    return authGuardSettingsCache.value
  }

  let nextSettings: AuthGuardSettings = {
    ...DEFAULT_AUTH_GUARD_SETTINGS,
  }
  try {
    const [rows] = await authDB.query(
      "SELECT `key`, `value` FROM system_configs WHERE `key` IN ('auth_registration_enabled', 'auth_rate_limit_window', 'auth_rate_limit_max_requests')",
    )
    if (Array.isArray(rows)) {
      for (const row of rows as any[]) {
        if (row?.key === 'auth_registration_enabled' && typeof row?.value === 'string') {
          nextSettings.registrationEnabled = row.value === 'true'
        }
        if (row?.key === 'auth_rate_limit_window' && row?.value) {
          nextSettings.windowSeconds = clampInteger(
            row.value,
            1,
            3600,
            DEFAULT_AUTH_GUARD_SETTINGS.windowSeconds,
          )
        }
        if (row?.key === 'auth_rate_limit_max_requests' && row?.value) {
          nextSettings.maxRequests = clampInteger(
            row.value,
            1,
            10000,
            DEFAULT_AUTH_GUARD_SETTINGS.maxRequests,
          )
        }
      }
    }
  } catch (error) {
    // If query fails (including missing table), apply safe defaults and avoid hammering DB.
    console.error('[auth] Failed to query system_configs for auth guard settings', error)
    nextSettings = { ...DEFAULT_AUTH_GUARD_SETTINGS }
  }

  authGuardSettingsCache = {
    value: nextSettings,
    expiresAtMs: nowMs + AUTH_GUARD_SETTINGS_TTL_MS,
  }
  return nextSettings
}

async function authEntryGuardMiddleware(
  req: AuthMiddlewareRequest,
  res: ServerResponse,
  requestIP: string,
): Promise<boolean> {
  const path = normalizeAuthPath(getAuthSubpath(req.url || ''))
  const settings = await getAuthGuardSettings()

  if (isSignUpPath(path)) {
    if (!settings.registrationEnabled) {
      sendNodeJSON(res, 403, {
        code: 403,
        msg: '注册已关闭',
      })
      return true
    }
  }

  if (isRateLimitedAuthPath(path)) {
    const limiterKey = buildRateLimitKey(req, path, requestIP)
    const check = checkAndHitRateLimit(limiterKey, settings.windowSeconds, settings.maxRequests)
    if (!check.allowed) {
      res.setHeader('Retry-After', String(check.retryAfterSeconds))
      sendNodeJSON(res, 429, {
        code: 429,
        msg: '请求过于频繁，请稍后重试',
      })
      return true
    }
  }

  return false
}

function getAuthSubpath(rawURL: string): string {
  const path = normalizeURLPath(rawURL)
  if (path === '/api/auth') return '/'
  if (path.startsWith('/api/auth/')) {
    return path.slice('/api/auth'.length)
  }
  return path
}

function normalizeAuthPath(rawPath: string): string {
  const path = String(rawPath || '').trim()
  if (!path) return '/'
  if (path.startsWith('/')) return path
  return `/${path}`
}

function isSignUpPath(path: string): boolean {
  return path === '/sign-up' || path === '/sign-up/email' || path.startsWith('/sign-up/')
}

function isRateLimitedAuthPath(path: string): boolean {
  return (
    path === '/sign-in' ||
    path === '/sign-up' ||
    path.startsWith('/sign-in/') ||
    path.startsWith('/sign-up/')
  )
}

function buildRateLimitKey(req: AuthMiddlewareRequest, path: string, requestIP: string): string {
  const bucket = path.startsWith('/sign-up/') ? 'signup' : 'signin'
  const ip = requestIP.trim() || String(req.socket?.remoteAddress || '').trim() || 'unknown'
  return `${bucket}:${ip}`
}

function clampInteger(value: unknown, min: number, max: number, fallback: number): number {
  const parsed = Number.parseInt(String(value), 10)
  if (!Number.isFinite(parsed)) {
    return fallback
  }
  if (parsed < min || parsed > max) {
    return fallback
  }
  return parsed
}

function checkAndHitRateLimit(
  key: string,
  windowSeconds: number,
  maxRequests: number,
): { allowed: boolean; retryAfterSeconds: number } {
  const nowMs = Date.now()
  const windowMs = windowSeconds * 1000

  let entry = authRateLimitStore.get(key)
  if (!entry || nowMs >= entry.resetAtMs) {
    entry = {
      count: 0,
      resetAtMs: nowMs + windowMs,
    }
  }

  if (entry.count >= maxRequests) {
    const retryAfterMs = entry.resetAtMs - nowMs
    authRateLimitStore.set(key, entry)
    return {
      allowed: false,
      retryAfterSeconds: Math.max(1, Math.ceil(retryAfterMs / 1000)),
    }
  }

  entry.count += 1
  authRateLimitStore.set(key, entry)

  authRateLimitCheckCounter += 1
  if (authRateLimitCheckCounter % 200 === 0) {
    cleanupExpiredRateLimitEntries(nowMs)
  }

  return {
    allowed: true,
    retryAfterSeconds: 0,
  }
}

function cleanupExpiredRateLimitEntries(nowMs: number) {
  for (const [key, entry] of authRateLimitStore.entries()) {
    if (entry.resetAtMs <= nowMs) {
      authRateLimitStore.delete(key)
    }
  }
}

function sendSuccess(reply: FastifyReply, data: unknown) {
  reply.status(200).send({
    code: 0,
    msg: 'success',
    data,
  })
}

function sendError(reply: FastifyReply, status: number, msg: string) {
  reply.status(status).send({
    code: status,
    msg,
  })
}

function getErrorStatus(error: unknown): number {
  const candidate = error as { status?: unknown; statusCode?: unknown }
  const status = Number(candidate.statusCode ?? candidate.status ?? 500)
  if (!Number.isInteger(status) || status < 400 || status > 599) {
    return 500
  }
  return status
}

function getHeaderValue(value: string | string[] | undefined): string {
  if (Array.isArray(value)) {
    return String(value[0] || '')
  }
  return String(value || '')
}

function sendNodeJSON(res: ServerResponse, status: number, payload: unknown) {
  if (!res.hasHeader('Content-Type')) {
    res.setHeader('Content-Type', 'application/json; charset=utf-8')
  }
  res.statusCode = status
  res.end(JSON.stringify(payload))
}

function sendNodeStatus(res: ServerResponse, status: number) {
  res.statusCode = status
  res.end()
}

function appendVaryHeader(res: ServerResponse, value: string) {
  const existing = res.getHeader('Vary')
  if (!existing) {
    res.setHeader('Vary', value)
    return
  }
  const values = Array.isArray(existing) ? existing : String(existing).split(',')
  const normalized = value.toLowerCase()
  if (values.some((item) => item.trim().toLowerCase() === normalized)) {
    return
  }
  res.setHeader(
    'Vary',
    [...values.map((item) => String(item).trim()).filter(Boolean), value].join(', '),
  )
}

function toHeaders(rawHeaders: InternalHeaders): Headers {
  const headers = new Headers()
  for (const [key, value] of Object.entries(rawHeaders)) {
    if (Array.isArray(value)) {
      for (const item of value) {
        headers.append(key, item)
      }
      continue
    }
    if (typeof value === 'string') {
      headers.set(key, value)
    }
  }
  return headers
}

function normalizeUsername(raw: unknown): string {
  const text = String(raw || '')
    .trim()
    .replace(/[^a-zA-Z0-9_-]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
  return text.slice(0, 50)
}

function normalizeAuthRole(raw: unknown): 'admin' | 'user' | null {
  const value = String(raw ?? 'user')
    .trim()
    .toLowerCase()
  if (value === 'admin' || value === 'user') {
    return value
  }
  return null
}

function normalizeOptionalText(raw: unknown, maxLength: number): string | null {
  const text = String(raw ?? '').trim()
  if (!text) return null
  return text.slice(0, maxLength)
}

function normalizeOptionalPositiveInteger(raw: unknown): number | null {
  if (raw == null || raw === '') return null
  const value = Number.parseInt(String(raw), 10)
  if (!Number.isFinite(value) || value <= 0) {
    return null
  }
  return value
}

function extractAuthUser(payload: unknown): InternalUserShape | null {
  const candidate = payload as any
  const user = (candidate?.user || candidate?.data?.user || candidate) as
    | InternalUserShape
    | undefined
  if (!user || typeof user.id !== 'string' || !user.id.trim()) {
    return null
  }
  return user
}

function extractErrorMessage(error: unknown, fallback: string): string {
  const err = error as any
  if (typeof err?.message === 'string' && err.message.trim()) return err.message
  if (typeof err?.error?.message === 'string' && err.error.message.trim()) return err.error.message
  if (typeof err?.data?.message === 'string' && err.data.message.trim()) return err.data.message
  if (typeof err?.data?.msg === 'string' && err.data.msg.trim()) return err.data.msg
  return fallback
}

function isDuplicateUserError(error: unknown): boolean {
  const message = extractErrorMessage(error, '').toLowerCase()
  return (
    message.includes('already exists') ||
    message.includes('already taken') ||
    message.includes('duplicate') ||
    message.includes('unique')
  )
}

async function findUserByUsernameOrEmail(
  username: string,
  email: string,
): Promise<InternalUserShape | null> {
  const [rows] = await authDB.query(
    'SELECT id, username, email, name, role FROM `user` WHERE username = ? OR email = ? LIMIT 1',
    [username, email],
  )
  if (!Array.isArray(rows) || rows.length === 0) {
    return null
  }
  const row = rows[0] as any
  if (typeof row?.id !== 'string' || !row.id.trim()) {
    return null
  }
  return {
    id: row.id,
    username: typeof row.username === 'string' ? row.username : null,
    email: typeof row.email === 'string' ? row.email : null,
    name: typeof row.name === 'string' ? row.name : null,
    role: typeof row.role === 'string' ? row.role : null,
  }
}

async function syncAuthUserRole(authUserID: string, role: 'admin' | 'user') {
  await updateAuthUserRole(authUserID, role)
}

async function updateAuthUserRole(authUserID: string, role: 'admin' | 'user'): Promise<boolean> {
  const [result] = await authDB.query('UPDATE `user` SET `role` = ? WHERE id = ? LIMIT 1', [
    role,
    authUserID,
  ])
  return Number((result as any)?.affectedRows || 0) > 0
}

async function updateAuthUserBanState(
  authUserID: string,
  banned: boolean,
  banReason: string | null,
  banExpiresIn: number | null,
): Promise<boolean> {
  const [result] = await authDB.query(
    'UPDATE `user` SET `banned` = ?, `banReason` = ?, `banExpires` = ?, `updatedAt` = ? WHERE id = ? LIMIT 1',
    [
      banned,
      banned ? banReason || 'No reason' : null,
      banned && banExpiresIn ? new Date(Date.now() + banExpiresIn * 1000) : null,
      new Date(),
      authUserID,
    ],
  )
  return Number((result as any)?.affectedRows || 0) > 0
}

async function deleteAuthUserSessions(authUserID: string): Promise<boolean> {
  const [rows] = await authDB.query('SELECT id FROM `user` WHERE id = ? LIMIT 1', [authUserID])
  if (!Array.isArray(rows) || rows.length === 0) {
    return false
  }
  await authDB.query('DELETE FROM `session` WHERE userId = ?', [authUserID])
  return true
}

async function removeAuthUserCascade(authUserID: string): Promise<boolean> {
  const conn = await authDB.getConnection()
  try {
    await conn.beginTransaction()
    const [rows] = await conn.query('SELECT id, email, username FROM `user` WHERE id = ? LIMIT 1', [
      authUserID,
    ])
    if (!Array.isArray(rows) || rows.length === 0) {
      await conn.rollback()
      return false
    }

    const row = rows[0] as any
    const email = typeof row?.email === 'string' ? row.email.trim() : ''
    const username = typeof row?.username === 'string' ? row.username.trim() : ''

    await conn.query('DELETE FROM `session` WHERE userId = ?', [authUserID])
    await conn.query('DELETE FROM `account` WHERE userId = ?', [authUserID])

    const verificationIdentifiers = [email, username, authUserID].filter(Boolean)
    if (verificationIdentifiers.length > 0) {
      await conn.query('DELETE FROM `verification` WHERE identifier IN (?)', [
        verificationIdentifiers,
      ])
    }

    await conn.query('DELETE FROM `user` WHERE id = ? LIMIT 1', [authUserID])
    await conn.commit()
    return true
  } catch (error) {
    await conn.rollback()
    throw error
  } finally {
    conn.release()
  }
}
