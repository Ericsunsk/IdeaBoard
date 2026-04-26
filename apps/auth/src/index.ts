import express, { type NextFunction, type Request, type Response } from 'express'
import { toNodeHandler } from 'better-auth/node'
import { auth, authDB } from './auth.js'
import { env } from './env.js'

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

const app = express()
const authRateLimitStore = new Map<string, MemoryRateLimitEntry>()
let authRateLimitCheckCounter = 0

app.get('/api/health', (_req, res) => {
  sendSuccess(res, { status: 'ok' })
})

app.use('/api/auth', authEntryGuardMiddleware)
app.all('/api/auth/*', toNodeHandler(auth))
app.use(express.json({ limit: '1mb' }))

const requireInternalKey = (req: Request, res: Response, next: NextFunction) => {
  if (!env.auth.internalKey) {
    next()
    return
  }
  const provided = String(req.header('x-internal-key') || '').trim()
  if (!provided || provided !== env.auth.internalKey) {
    sendError(res, 401, 'unauthorized internal request')
    return
  }
  next()
}

app.get('/internal/session', requireInternalKey, async (req, res) => {
  try {
    const session = await auth.api.getSession({
      headers: toHeaders(req),
    })
    const user = extractAuthUser(session)
    if (!user) {
      sendError(res, 401, 'unauthorized')
      return
    }
    sendSuccess(res, {
      session: {
        auth_user_id: user.id,
        username: normalizeUsername(user.username || user.name || ''),
        email: String(user.email || ''),
      },
    })
  } catch (error) {
    sendError(res, 500, extractErrorMessage(error, 'failed to validate session'))
  }
})

app.post('/internal/users/ensure', requireInternalKey, async (req, res) => {
  const username = normalizeUsername(req.body?.username)
  const password = String(req.body?.password || '')
  const allowExisting = Boolean(req.body?.allow_existing)
  const role = normalizeAuthRole(req.body?.role)

  if (!username) {
    sendError(res, 400, 'username is required')
    return
  }
  if (password.length < 6 || password.length > 50) {
    sendError(res, 400, 'password length must be between 6 and 50')
    return
  }
  if (!role) {
    sendError(res, 400, 'role must be admin or user')
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
      sendError(res, 500, 'auth user creation succeeded but user payload is empty')
      return
    }
    await syncAuthUserRole(user.id, role)
    sendSuccess(res, {
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
          sendSuccess(res, {
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
        sendError(res, 500, extractErrorMessage(lookupError, 'failed to locate existing user'))
        return
      }
    }
    sendError(res, 400, extractErrorMessage(error, 'failed to ensure user'))
  }
})

if (isProductionRuntime() && !env.auth.internalKey) {
  throw new Error('AUTH_INTERNAL_KEY is required when NODE_ENV=production')
}

app.listen(env.auth.port, env.auth.host, () => {
  if (!env.auth.internalKey) {
    console.warn('[auth] AUTH_INTERNAL_KEY is empty; internal endpoints are not protected')
  }
  console.log(`[auth] Better Auth service listening on http://${env.auth.host}:${env.auth.port}`)
})

function isProductionRuntime(): boolean {
  const value = String(process.env.NODE_ENV || process.env.APP_ENV || '').trim().toLowerCase()
  return value === 'production' || value === 'prod' || value === 'release'
}

function authEntryGuardMiddleware(req: Request, res: Response, next: NextFunction) {
  const path = normalizeAuthPath(req.path)

  if (isSignUpPath(path) && !env.auth.registrationEnabled) {
    sendError(res, 403, '注册已关闭')
    return
  }

  if (isRateLimitedAuthPath(path)) {
    const limiterKey = buildRateLimitKey(req, path)
    const check = checkAndHitRateLimit(limiterKey)
    if (!check.allowed) {
      res.setHeader('Retry-After', String(check.retryAfterSeconds))
      sendError(res, 429, '请求过于频繁，请稍后重试')
      return
    }
  }

  next()
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
  return path === '/sign-in' || path === '/sign-up' || path.startsWith('/sign-in/') || path.startsWith('/sign-up/')
}

function buildRateLimitKey(req: Request, path: string): string {
  const bucket = path.startsWith('/sign-up/') ? 'signup' : 'signin'
  const ip = getRequestIP(req)
  return `${bucket}:${ip}`
}

function getRequestIP(req: Request): string {
  const forwardedFor = String(req.headers['x-forwarded-for'] || '').trim()
  if (forwardedFor) {
    const candidate = forwardedFor.split(',')[0]?.trim()
    if (candidate) return candidate
  }
  const realIP = String(req.headers['x-real-ip'] || '').trim()
  if (realIP) return realIP
  const expressIP = String(req.ip || '').trim()
  if (expressIP) return expressIP
  const remoteIP = String(req.socket?.remoteAddress || '').trim()
  if (remoteIP) return remoteIP
  return 'unknown'
}

function checkAndHitRateLimit(key: string): { allowed: boolean; retryAfterSeconds: number } {
  const nowMs = Date.now()
  const windowMs = env.auth.rateLimitWindowSeconds * 1000
  const maxRequests = env.auth.rateLimitMaxRequests

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

function sendSuccess(res: Response, data: unknown) {
  res.status(200).json({
    code: 0,
    msg: 'success',
    data,
  })
}

function sendError(res: Response, status: number, msg: string) {
  res.status(status).json({
    code: status,
    msg,
  })
}

function toHeaders(req: Request): Headers {
  const headers = new Headers()
  for (const [key, value] of Object.entries(req.headers)) {
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

function extractAuthUser(payload: unknown): InternalUserShape | null {
  const candidate = payload as any
  const user = (candidate?.user || candidate?.data?.user || candidate) as InternalUserShape | undefined
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

async function findUserByUsernameOrEmail(username: string, email: string): Promise<InternalUserShape | null> {
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
  await authDB.query('UPDATE `user` SET `role` = ? WHERE id = ? LIMIT 1', [role, authUserID])
}
