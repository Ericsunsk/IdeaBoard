import { ref } from 'vue'

export type AdminConfirmIntent = 'warn' | 'danger'

export type AdminConfirmRequest = {
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  onConfirm: () => void | Promise<void>
}

export type AdminConfirmOptions = {
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  onConfirm?: () => void | Promise<void>
}

export type AdminConfirmPort = {
  confirm: (options: AdminConfirmOptions) => Promise<boolean>
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
  function confirm(options: AdminConfirmOptions): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
      activeAdminConfirm.value = {
        open: true,
        title: options.title,
        message: options.message,
        confirmLabel: options.confirmLabel,
        intent: options.intent,
        loading: false,
        confirm: async () => {
          if (!activeAdminConfirm.value) return
          activeAdminConfirm.value.loading = true
          try {
            if (options.onConfirm) {
              await options.onConfirm()
            }
            activeAdminConfirm.value = null
            resolve(true)
          } catch (error) {
            if (activeAdminConfirm.value) {
              activeAdminConfirm.value.loading = false
            }
            reject(error)
            throw error
          }
        },
        cancel: () => {
          activeAdminConfirm.value = null
          resolve(false)
        },
      }
    })
  }

  return {
    confirm,
  }
}
