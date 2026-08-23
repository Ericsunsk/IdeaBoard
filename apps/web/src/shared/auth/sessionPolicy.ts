import {
  ADMIN_LOGIN_PATH,
  ADMIN_ROOT_PATH,
  HOME_PATH,
  SHARE_JOIN_PATH,
  USER_BOARDS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_ROOT_PATH,
  isPathUnderRoute,
} from '../constants/routePaths'

const SAFE_REDIRECT_PREFIXES = [USER_ROOT_PATH, ADMIN_ROOT_PATH, SHARE_JOIN_PATH] as const
const AUTH_REDIRECT_PATHS = [USER_LOGIN_PATH, USER_REGISTER_PATH, ADMIN_LOGIN_PATH] as const
const REDIRECT_BASE_URL = 'http://ideaboard.local'

export type AuthFailureActionKind = 'ignore' | 'clear-only' | 'redirect-login' | 'redirect-home'

export type AuthFailureAction =
  | { kind: 'ignore' }
  | { kind: 'clear-only' }
  | { kind: 'redirect-login'; targetPath: string }
  | { kind: 'redirect-home'; targetPath: typeof HOME_PATH }

export type AuthFailureContext = {
  status?: number
  payload?: unknown
  requestUrl: string
  currentPathname?: string
  currentFullPath?: string
}

export type SafeInternalRedirectPath = {
  pathname: string
  fullPath: string
}

function getRedirectPathnameInput(path: string): string {
  const splitIndex = path.search(/[?#]/)
  return splitIndex === -1 ? path : path.slice(0, splitIndex)
}

function hasDotPathSegment(pathname: string): boolean {
  const segments = pathname.split('/')
  return segments.some((segment) => {
    try {
      const decodedSegment = decodeURIComponent(segment)
      return decodedSegment === '.' || decodedSegment === '..'
    } catch {
      return true
    }
  })
}

export function normalizeSafeInternalRedirectPath(
  path: string,
  baseUrl = REDIRECT_BASE_URL,
): SafeInternalRedirectPath | null {
  const redirectPath = path.trim()
  if (
    !redirectPath ||
    !redirectPath.startsWith('/') ||
    redirectPath.startsWith('//') ||
    redirectPath.includes('\\') ||
    /%5c/i.test(redirectPath)
  ) {
    return null
  }

  const inputPathname = getRedirectPathnameInput(redirectPath)
  if (hasDotPathSegment(inputPathname)) {
    return null
  }

  let redirectUrl: URL
  try {
    redirectUrl = new URL(redirectPath, baseUrl)
  } catch {
    return null
  }

  if (redirectUrl.origin !== new URL(baseUrl).origin) {
    return null
  }

  const redirectPathname = redirectUrl.pathname
  if (redirectPathname.includes('\\') || /%5c/i.test(redirectPathname)) {
    return null
  }

  if (!SAFE_REDIRECT_PREFIXES.some((prefix) => isPathUnderRoute(redirectPathname, prefix))) {
    return null
  }

  if (AUTH_REDIRECT_PATHS.some((authPath) => isPathUnderRoute(redirectPathname, authPath))) {
    return null
  }

  return {
    pathname: redirectPathname,
    fullPath: redirectPathname + redirectUrl.search + redirectUrl.hash,
  }
}

export function isSafeInternalRedirectPath(path: string, baseUrl = REDIRECT_BASE_URL): boolean {
  return normalizeSafeInternalRedirectPath(path, baseUrl) !== null
}

function getPayloadCode(payload: unknown): unknown {
  return payload && typeof payload === 'object' ? (payload as { code?: unknown }).code : undefined
}

function isAuthRequest(requestUrl: string) {
  return requestUrl.includes('/auth/')
}

function isSessionProbeRequest(requestUrl: string) {
  return requestUrl.includes('/user/info')
}

function isAdminApiRequest(requestUrl: string) {
  return requestUrl.includes('/admin/')
}

export function buildLoginRedirectPath(currentPathname: string, currentFullPath: string) {
  const isAdminPath = isPathUnderRoute(currentPathname, ADMIN_ROOT_PATH)
  const loginPath = isAdminPath ? ADMIN_LOGIN_PATH : USER_LOGIN_PATH
  const postLoginPath =
    normalizeSafeInternalRedirectPath(currentFullPath)?.fullPath ??
    (isAdminPath ? ADMIN_ROOT_PATH : USER_BOARDS_PATH)
  return loginPath + '?redirect=' + encodeURIComponent(postLoginPath)
}

export function resolveAuthFailureAction(context: AuthFailureContext): AuthFailureAction {
  const { status, payload, requestUrl, currentPathname = '', currentFullPath = '' } = context
  const code = getPayloadCode(payload) ?? status
  const isOnAdminPage = isPathUnderRoute(currentPathname, ADMIN_ROOT_PATH)

  if (code === 403 && isAdminApiRequest(requestUrl) && isOnAdminPage) {
    return { kind: 'redirect-home', targetPath: HOME_PATH }
  }

  if (code === 401 && isSessionProbeRequest(requestUrl)) {
    return { kind: 'clear-only' }
  }

  if (code !== 401 || isAuthRequest(requestUrl)) {
    return { kind: 'ignore' }
  }

  if (!currentPathname || !currentFullPath) {
    return { kind: 'clear-only' }
  }

  return {
    kind: 'redirect-login',
    targetPath: buildLoginRedirectPath(currentPathname, currentFullPath),
  }
}
