import { computed, inject, provide, readonly, ref, type InjectionKey } from 'vue'

export type AuthOverlayMode = 'login' | 'register'

export function createAuthOverlay() {
  const mode = ref<AuthOverlayMode | null>(null)

  function open(nextMode: AuthOverlayMode = 'login') {
    mode.value = nextMode
  }

  function close() {
    mode.value = null
  }

  return {
    viewModel: {
      mode: readonly(mode),
      visible: computed(() => mode.value !== null),
    },
    commands: { close, open },
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
