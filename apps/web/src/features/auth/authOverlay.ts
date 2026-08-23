import { computed, inject, provide, readonly, ref, type InjectionKey } from 'vue'
import { normalizeSafeInternalRedirectPath } from '../../shared/auth/sessionPolicy'

export type AuthOverlayMode = 'login' | 'register'

const POST_AUTH_PATH_KEY = 'ideaboard:auth:post-auth-path'

function authSessionStorage(): Storage | null {
  if (typeof window === 'undefined') return null
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function persistPostAuthPath(path: string | null): void {
  const storage = authSessionStorage()
  if (!storage) return
  try {
    if (path) {
      storage.setItem(POST_AUTH_PATH_KEY, path)
    } else {
      storage.removeItem(POST_AUTH_PATH_KEY)
    }
  } catch {
    // Authentication must remain available when sessionStorage is unavailable.
  }
}

function readPostAuthPath(): string | null {
  const storage = authSessionStorage()
  if (!storage) return null
  try {
    const path = storage.getItem(POST_AUTH_PATH_KEY)
    if (!path) return null
    const normalized = normalizeSafeInternalRedirectPath(path)?.fullPath ?? null
    if (!normalized) storage.removeItem(POST_AUTH_PATH_KEY)
    return normalized
  } catch {
    return null
  }
}

export function createAuthOverlay() {
  const mode = ref<AuthOverlayMode | null>(null)

  function open(nextMode: AuthOverlayMode = 'login', postAuthPath?: string) {
    if (postAuthPath !== undefined) {
      persistPostAuthPath(normalizeSafeInternalRedirectPath(postAuthPath)?.fullPath ?? null)
    }
    mode.value = nextMode
  }

  function close() {
    mode.value = null
  }

  function clearPostAuthPath() {
    persistPostAuthPath(null)
  }

  function consumePostAuthPath(): string | null {
    const path = readPostAuthPath()
    persistPostAuthPath(null)
    return path
  }

  return {
    viewModel: {
      mode: readonly(mode),
      visible: computed(() => mode.value !== null),
    },
    commands: { clearPostAuthPath, close, consumePostAuthPath, open },
  }
}

export type AuthOverlay = ReturnType<typeof createAuthOverlay>

const authOverlayKey: InjectionKey<AuthOverlay> = Symbol('auth-overlay')

export function provideAuthOverlay(overlay: AuthOverlay) {
  provide(authOverlayKey, overlay)
}

export function useAuthOverlay(): AuthOverlay {
  const overlay = inject(authOverlayKey)
  if (!overlay) {
    throw new Error('Auth overlay is not installed by User Shell')
  }
  return overlay
}
