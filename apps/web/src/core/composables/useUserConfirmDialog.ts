import { getCurrentScope, onScopeDispose, ref, type Ref } from 'vue'

export type UserConfirmRequest = {
  title: string
  message: string
  confirmLabel?: string | undefined
  intent?: 'primary' | 'danger' | undefined
}

export function useUserConfirmDialog(): {
  visible: Ref<boolean>
  title: Ref<string>
  message: Ref<string>
  confirmLabel: Ref<string>
  intent: Ref<'primary' | 'danger'>
  loading: Ref<boolean>
  request: (request: UserConfirmRequest, action: () => void | Promise<void>) => void
  confirmAsync: (request: UserConfirmRequest) => Promise<boolean>
  confirm: () => Promise<void>
  close: () => void
  dispose: () => void
} {
  const visible = ref(false)
  const title = ref('')
  const message = ref('')
  const confirmLabel = ref('Confirm')
  const intent = ref<'primary' | 'danger'>('primary')
  const loading = ref(false)
  let pendingAction: (() => void | Promise<void>) | null = null
  let pendingCancel: (() => void) | null = null

  function request(next: UserConfirmRequest, action: () => void | Promise<void>) {
    title.value = next.title
    message.value = next.message
    confirmLabel.value = next.confirmLabel ?? 'Confirm'
    intent.value = next.intent ?? 'primary'
    pendingAction = action
    pendingCancel = null
    visible.value = true
  }

  function confirmAsync(next: UserConfirmRequest): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      title.value = next.title
      message.value = next.message
      confirmLabel.value = next.confirmLabel ?? 'Confirm'
      intent.value = next.intent ?? 'primary'
      pendingAction = () => {
        resolve(true)
      }
      pendingCancel = () => {
        resolve(false)
      }
      visible.value = true
    })
  }

  function close() {
    if (loading.value) return
    visible.value = false
    if (pendingCancel) {
      pendingCancel()
      pendingCancel = null
    }
    pendingAction = null
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
