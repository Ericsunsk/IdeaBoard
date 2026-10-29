import { getCurrentScope, onScopeDispose, ref, type Ref } from 'vue'

export type ConfirmIntent = 'primary' | 'warn' | 'danger'
export type ConfirmSurface = 'user' | 'admin'

export type ConfirmRequest = {
  title: string
  message: string
  confirmLabel?: string | undefined
  intent?: ConfirmIntent | undefined
}

export type ConfirmOptions = {
  title: string
  message: string
  confirmLabel: string
  intent: ConfirmIntent
  onConfirm?: (() => void | Promise<void>) | undefined
}

export type ConfirmPort = {
  confirm: (options: ConfirmOptions) => Promise<boolean>
}

export type ConfirmState = {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  intent: ConfirmIntent
  loading: boolean
  confirm: () => Promise<void>
  cancel: () => void
}

export const activeConfirm = ref<ConfirmState | null>(null)

/**
 * Global promise-based confirm (operator surfaces mount one `ConfirmDialog`
 * with `surface="admin"` and drive it through this singleton).
 *
 * Semantics: `confirm()` runs `onConfirm` under a busy lock and settles
 * `true`; `cancel()` settles `false`. A throwing `onConfirm` keeps the
 * dialog open, resets the busy flag, and rejects so the dialog can toast.
 */
export function useConfirm(): ConfirmPort {
  function confirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
      activeConfirm.value = {
        open: true,
        title: options.title,
        message: options.message,
        confirmLabel: options.confirmLabel,
        intent: options.intent,
        loading: false,
        confirm: async () => {
          if (!activeConfirm.value) return
          if (activeConfirm.value.loading) return
          activeConfirm.value.loading = true
          try {
            if (options.onConfirm) {
              await options.onConfirm()
            }
            activeConfirm.value = null
            resolve(true)
          } catch (error) {
            if (activeConfirm.value) {
              activeConfirm.value.loading = false
            }
            reject(error)
            throw error
          }
        },
        cancel: () => {
          activeConfirm.value = null
          resolve(false)
        },
      }
    })
  }

  return {
    confirm,
  }
}

export function useConfirmDialog(): {
  visible: Ref<boolean>
  title: Ref<string>
  message: Ref<string>
  confirmLabel: Ref<string>
  intent: Ref<ConfirmIntent>
  loading: Ref<boolean>
  request: (request: ConfirmRequest, action: () => void | Promise<void>) => void
  confirmAsync: (request: ConfirmRequest) => Promise<boolean>
  confirm: () => Promise<void>
  close: () => void
  dispose: () => void
} {
  const visible = ref(false)
  const title = ref('')
  const message = ref('')
  const confirmLabel = ref('Confirm')
  const intent = ref<ConfirmIntent>('primary')
  const loading = ref(false)
  let pendingAction: (() => void | Promise<void>) | null = null
  let pendingCancel: (() => void) | null = null

  function request(
    next: ConfirmRequest,
    action: () => void | Promise<void>,
    onCancel: (() => void) | null = null,
  ) {
    title.value = next.title
    message.value = next.message
    confirmLabel.value = next.confirmLabel ?? 'Confirm'
    intent.value = next.intent ?? 'primary'
    pendingAction = action
    pendingCancel = onCancel
    visible.value = true
  }

  function confirmAsync(next: ConfirmRequest): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      request(
        next,
        () => resolve(true),
        () => resolve(false),
      )
    })
  }

  function close() {
    if (loading.value) return
    dispose()
  }

  async function confirm() {
    if (!pendingAction || loading.value) return
    loading.value = true
    try {
      await pendingAction()
      visible.value = false
      pendingAction = null
      pendingCancel = null
    } finally {
      loading.value = false
    }
  }

  // A parent unmount must never orphan a pending confirmAsync: settle it as
  // dismissed so awaiters always resolve.
  function dispose() {
    visible.value = false
    if (pendingCancel) {
      pendingCancel()
      pendingCancel = null
    }
    pendingAction = null
  }

  if (getCurrentScope()) onScopeDispose(dispose)

  return {
    visible,
    title,
    message,
    confirmLabel,
    intent,
    loading,
    request,
    confirmAsync,
    confirm,
    close,
    dispose,
  }
}
