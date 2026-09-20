import { ref } from 'vue'

export type AdminConfirmIntent = 'warn' | 'danger'

export type AdminConfirmRequest = {
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  onConfirm: () => void | Promise<void>
}

export type AdminConfirmPort = {
  request: (request: AdminConfirmRequest) => void
}

export type AdminConfirmState = {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  loading: boolean
  confirm: () => Promise<void>
  cancel: () => void
}

export const activeAdminConfirm = ref<AdminConfirmState | null>(null)

export function useAdminConfirm(): AdminConfirmPort {
  return {
    request(request) {
      activeAdminConfirm.value = {
        open: true,
        title: request.title,
        message: request.message,
        confirmLabel: request.confirmLabel,
        intent: request.intent,
        loading: false,
        confirm: async () => {
          if (!activeAdminConfirm.value) return
          activeAdminConfirm.value.loading = true
          try {
            await request.onConfirm()
          } finally {
            activeAdminConfirm.value = null
          }
        },
        cancel: () => {
          activeAdminConfirm.value = null
        },
      }
    },
  }
}
