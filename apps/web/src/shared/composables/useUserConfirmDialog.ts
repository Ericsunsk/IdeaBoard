import { ref, type Ref } from 'vue'

export type UserConfirmRequest = {
  title: string
  message: string
  confirmLabel?: string
  intent?: 'primary' | 'danger'
}

export function useUserConfirmDialog(): {
  visible: Ref<boolean>
  title: Ref<string>
  message: Ref<string>
  confirmLabel: Ref<string>
  intent: Ref<'primary' | 'danger'>
  loading: Ref<boolean>
  request: (request: UserConfirmRequest, action: () => void | Promise<void>) => void
  confirm: () => Promise<void>
  close: () => void
} {
  const visible = ref(false)
  const title = ref('')
  const message = ref('')
  const confirmLabel = ref('Confirm')
  const intent = ref<'primary' | 'danger'>('primary')
  const loading = ref(false)
  let pendingAction: (() => void | Promise<void>) | null = null

  function request(next: UserConfirmRequest, action: () => void | Promise<void>) {
    title.value = next.title
    message.value = next.message
    confirmLabel.value = next.confirmLabel ?? 'Confirm'
    intent.value = next.intent ?? 'primary'
    pendingAction = action
    visible.value = true
  }

  function close() {
    if (loading.value) return
    visible.value = false
    pendingAction = null
  }

  async function confirm() {
    if (!pendingAction || loading.value) return
    loading.value = true
    try {
      await pendingAction()
      visible.value = false
      pendingAction = null
    } finally {
      loading.value = false
    }
  }

  return {
    visible,
    title,
    message,
    confirmLabel,
    intent,
    loading,
    request,
    confirm,
    close,
  }
}
