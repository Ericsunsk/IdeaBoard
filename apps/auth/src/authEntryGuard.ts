import type { IncomingMessage, ServerResponse } from 'node:http'
import type { AuthGuardSettings } from './authGuardSettings.js'
import type { MailReadiness } from './mailConfig.js'
import {
  getAuthSubpath,
  isRateLimitedAuthPath,
  isSignUpPath,
  normalizeAuthPath,
  normalizeURLPath,
} from './authUtils.js'
import {
  appendVaryHeader,
  getHeaderValue,
  sendNodeJSON,
  sendNodeStatus,
} from './httpResponse.js'

type MemoryRateLimitEntry = {
  count: number
  resetAtMs: number
}

export type AuthMiddlewareRequest = IncomingMessage & {
  socket: IncomingMessage['socket']
}

export type AuthNodeHandler = (
  req: IncomingMessage,
  res: ServerResponse,
) => Promise<void> | void

export type AuthEntryGuardOptions = {
  trustedOrigins: string[]
  getAuthGuardSettings: () => Promise<AuthGuardSettings>
  authHandler: AuthNodeHandler
  getMailReadiness?: () => Promise<MailReadiness>
}

export type AuthEntryGuard = {
  handle(req: AuthMiddlewareRequest, res: ServerResponse, requestIP: string): Promise<void>
}

export function createAuthEntryGuard(options: AuthEntryGuardOptions): AuthEntryGuard {
  const allowedAuthOrigins = new Set(
    options.trustedOrigins.map(normalizeCORSOrigin).filter(Boolean),
  )
  const authRateLimitStore = new Map<string, MemoryRateLimitEntry>()
  let authRateLimitCheckCounter = 0

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
      cleanupExpiredRateLimitEntries(authRateLimitStore, nowMs)
    }

    return {
      allowed: true,
      retryAfterSeconds: 0,
    }
  }

  async function authEntryGuardMiddleware(
    req: AuthMiddlewareRequest,
    res: ServerResponse,
    requestIP: string,
  ): Promise<boolean> {
    const path = normalizeAuthPath(getAuthSubpath(req.url || ''))
    const settings = await options.getAuthGuardSettings()

    if (isSignUpPath(path)) {
      if (!settings.registrationEnabled) {
        sendNodeJSON(res, 403, {
          code: 403,
          msg: '注册已关闭',
        })
        return true
      }
    }

    if (options.getMailReadiness && requiresMailReadiness(path)) {
      let readiness: MailReadiness
      try {
        readiness = await options.getMailReadiness()
      } catch (error) {
        console.error('[auth] Failed to read mail readiness', error)
        sendNodeJSON(res, 503, { code: 503, msg: '邮件服务暂不可用' })
        return true
      }
      if (readiness !== 'ready') {
        sendNodeJSON(res, 503, { code: 503, msg: '邮件服务尚未就绪' })
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

  return {
    async handle(req, res, requestIP) {
      const corsHandled = authCORSMiddleware(req, res, allowedAuthOrigins)
      if (corsHandled) {
        return
      }

      const guardHandled = await authEntryGuardMiddleware(req, res, requestIP)
      if (guardHandled) {
        return
      }

      await options.authHandler(req, res)
    },
  }
}

function requiresMailReadiness(path: string): boolean {
  return (
    isSignUpPath(path) ||
    path === '/request-password-reset' ||
    path === '/email-otp/request-password-reset' ||
    path === '/forget-password/email-otp' ||
    path === '/email-otp/reset-password' ||
    path === '/email-otp/request-email-change' ||
    path === '/email-otp/change-email'
  )
}

export function isAuthRoute(url: string): boolean {
  const path = normalizeURLPath(url)
  return path === '/api/auth' || path.startsWith('/api/auth/')
}

function authCORSMiddleware(
  req: IncomingMessage,
  res: ServerResponse,
  allowedAuthOrigins: Set<string>,
): boolean {
  const origin = getHeaderValue(req.headers.origin).trim()
  if (!origin) {
    return false
  }

  const isAllowed = allowedAuthOrigins.has(normalizeCORSOrigin(origin))
  if (!isAllowed) {
    sendNodeStatus(res, 403)
    return true
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

function buildRateLimitKey(req: AuthMiddlewareRequest, path: string, requestIP: string): string {
  const bucket = isSignUpPath(path) ? 'signup' : 'signin'
  const ip = requestIP.trim() || String(req.socket?.remoteAddress || '').trim() || 'unknown'
  return `${bucket}:${ip}`
}

function cleanupExpiredRateLimitEntries(
  authRateLimitStore: Map<string, MemoryRateLimitEntry>,
  nowMs: number,
) {
  for (const [key, entry] of authRateLimitStore.entries()) {
    if (entry.resetAtMs <= nowMs) {
      authRateLimitStore.delete(key)
    }
  }
}
