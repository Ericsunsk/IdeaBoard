const HTTP_URL_RE = /^https?:\/\//i
const PASSTHROUGH_URL_RE = /^(data|blob):/i

function trimTrailingSlashes(value: string): string {
  return value.replace(/\/+$/, '')
}

function normalizeBaseURL(value: string | undefined, fallback: string): string {
  const trimmed = (value || '').trim()
  if (!trimmed) return fallback
  if (trimmed === '/') return ''
  return trimTrailingSlashes(trimmed)
}

function defaultAuthBaseURL(): string {
  if (typeof window !== 'undefined') return window.location.origin
  return 'http://localhost:5173'
}

export const API_BASE_URL = normalizeBaseURL(import.meta.env.VITE_API_BASE_URL, '/api')
export const AUTH_BASE_URL = normalizeBaseURL(
  import.meta.env.VITE_AUTH_BASE_URL,
  defaultAuthBaseURL(),
)

function deriveAssetBaseURL(): string {
  const explicit = normalizeBaseURL(import.meta.env.VITE_PUBLIC_ASSET_BASE_URL, '')
  if (explicit) return explicit
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
    return ''
  }
}

export const PUBLIC_ASSET_BASE_URL = deriveAssetBaseURL()

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

export function resolveAssetUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed || HTTP_URL_RE.test(trimmed) || PASSTHROUGH_URL_RE.test(trimmed)) return raw
  if (trimmed.startsWith('/uploads/') && PUBLIC_ASSET_BASE_URL) {
    return joinRuntimeURL(PUBLIC_ASSET_BASE_URL, trimmed)
  }
  return raw
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Object.prototype.toString.call(value) === '[object Object]'
}

export function normalizeRuntimeMediaRefs<T>(value: T): T {
  if (typeof value === 'string') {
    return resolveAssetUrl(value) as T
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
