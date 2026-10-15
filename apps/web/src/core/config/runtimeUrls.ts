const HTTP_URL_RE = /^https?:\/\//i
const PASSTHROUGH_URL_RE = /^(data|blob):/i

function trimTrailingSlashes(value: string): string {
  return value.replace(/\/+$/, '')
}

function isValidRuntimeBaseURL(value: string): boolean {
  if (value.startsWith('/')) return true
  try {
    const parsed = new URL(value)
    return (
      (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      Boolean(parsed.host) &&
      !parsed.username &&
      !parsed.password &&
      !parsed.search &&
      !parsed.hash
    )
  } catch {
    return false
  }
}

function normalizeBaseURL(value: string | undefined, fallback: string): string {
  const trimmed = (value || '').trim()
  if (!trimmed) return fallback
  if (trimmed === '/') return ''
  if (!isValidRuntimeBaseURL(trimmed)) return fallback
  return trimTrailingSlashes(trimmed)
}

function isLoopbackHostname(hostname: string): boolean {
  const normalized = hostname.trim().toLowerCase()
  return (
    normalized === 'localhost' ||
    normalized === '127.0.0.1' ||
    normalized === '[::1]' ||
    normalized === '::1'
  )
}

function defaultAuthBaseURL(): string {
  if (typeof window !== 'undefined') return window.location.origin
  return ''
}

// Production is served behind the same-origin gateway. Ignore legacy absolute
// API overrides in production so a stale Pages environment variable cannot
// silently reintroduce browser CORS requests. Local/dev builds may still point
// at a remote API when explicitly configured for development.
const runtimeEnv = import.meta.env
const isProductionBuild = runtimeEnv.PROD === true || runtimeEnv.MODE === 'production'
const configuredAPIBaseURL = normalizeBaseURL(runtimeEnv.VITE_API_BASE_URL, '/api')
export const API_BASE_URL = isProductionBuild ? '/api' : configuredAPIBaseURL

function deriveAPIOriginBaseURL(fallback: string): string {
  if (!HTTP_URL_RE.test(API_BASE_URL)) return ''

  try {
    const url = new URL(API_BASE_URL)
    const pathname = trimTrailingSlashes(url.pathname)
    if (pathname === '/api' || pathname.endsWith('/api')) {
      url.pathname = pathname.slice(0, -'/api'.length) || '/'
    }
    url.search = ''
    url.hash = ''
    return trimTrailingSlashes(url.toString())
  } catch {
    return fallback
  }
}

export const AUTH_BASE_URL = deriveAPIOriginBaseURL(defaultAuthBaseURL())

export function joinRuntimeURL(base: string, path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  if (!base) return normalizedPath
  if (HTTP_URL_RE.test(base)) {
    return new URL(normalizedPath.slice(1), `${trimTrailingSlashes(base)}/`).toString()
  }
  return `${trimTrailingSlashes(base)}${normalizedPath}`
}

export function resolveApiUrl(path: string): string {
  return joinRuntimeURL(API_BASE_URL, path)
}

export function resolveAuthUrl(path: string): string {
  return joinRuntimeURL(AUTH_BASE_URL, path)
}

/**
 * Normalize a delivery URL for browser use. Delivery runs behind the
 * same-origin gateway, so absolute http(s), data:, and blob: URLs pass
 * through and relative paths are already fetchable: the only normalization
 * left is trimming. Prefer canonicalizeRuntimeUploadRef when the input may
 * be a non-canonical (absolute CDN/loopback) upload ref.
 */
export function resolveAssetUrl(raw: string): string {
  return raw.trim()
}

export function canonicalizeRuntimeUploadRef(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed || PASSTHROUGH_URL_RE.test(trimmed)) return raw
  if (trimmed.startsWith('/uploads/')) return trimmed
  if (!HTTP_URL_RE.test(trimmed)) return raw

  try {
    const url = new URL(trimmed)
    if (!url.pathname.startsWith('/uploads/')) return raw
    const isPublicAssetOrigin =
      typeof window !== 'undefined' && url.origin === window.location.origin
    if (!isPublicAssetOrigin && !isLoopbackHostname(url.hostname)) return raw
    return `${url.pathname}${url.search}`
  } catch {
    return raw
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Object.prototype.toString.call(value) === '[object Object]'
}

export function normalizeRuntimeMediaRefs<T>(value: T): T {
  if (typeof value === 'string') {
    return resolveAssetUrl(canonicalizeRuntimeUploadRef(value)) as T
  }
  if (Array.isArray(value)) {
    return value.map((item) => normalizeRuntimeMediaRefs(item)) as T
  }
  if (isPlainObject(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, normalizeRuntimeMediaRefs(item)]),
    ) as T
  }
  return value
}
