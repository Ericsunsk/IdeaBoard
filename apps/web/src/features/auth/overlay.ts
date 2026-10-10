import { computed, inject, provide, readonly, ref, type InjectionKey } from 'vue'
import { normalizeSafeInternalRedirectPath } from '../../core/auth/sessionPolicy'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../core/utils/storage'

export type AuthOverlayMode = 'login' | 'register'

const POST_AUTH_PATH_KEY = 'ideaboard:auth:post-auth-path'

function persistPostAuthPath(path: string | null): void {
  try {
    if (path) {
      writeSessionStorageItem(POST_AUTH_PATH_KEY, path)
    } else {
      removeSessionStorageItem(POST_AUTH_PATH_KEY)
    }
  } catch {
    // Authentication must remain available when sessionStorage is unavailable.
  }
}

function readPostAuthPath(): string | null {
  try {
    const path = readSessionStorageItem(POST_AUTH_PATH_KEY)
    if (!path) return null
    const normalized = normalizeSafeInternalRedirectPath(path)?.fullPath ?? null
    if (!normalized) removeSessionStorageItem(POST_AUTH_PATH_KEY)
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
