import { ref } from 'vue'

export type AdminConfirmIntent = 'warn' | 'danger'

// Operator reason capture (issue 808): control mutations require an
// operator note. When present, the dialog renders a reason field and the
// caller reads it via readConfirmReason() after confirm() resolves true.
export type AdminConfirmReasonSpec = {
  required: boolean
  label?: string | undefined
  placeholder?: string | undefined
}

export type AdminConfirmRequest = {
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  reason?: AdminConfirmReasonSpec | undefined
  onConfirm: () => void | Promise<void>
}

export type AdminConfirmOptions = {
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  reason?: AdminConfirmReasonSpec | undefined
  onConfirm?: () => void | Promise<void>
}

export type AdminConfirmPort = {
  confirm: (options: AdminConfirmOptions) => Promise<boolean>
  // Optional so existing callers/mocks without reason capture keep working;
  // commands that require an operator reason must provide a readReason.
  readReason?: () => string
}

export type AdminConfirmState = {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  intent: AdminConfirmIntent
  reason: string
  reasonEnabled: boolean
  reasonRequired: boolean
  reasonLabel: string
  reasonPlaceholder: string
  loading: boolean
  confirm: () => Promise<void>
  cancel: () => void
  setReason: (value: string) => void
}

export const activeAdminConfirm = ref<AdminConfirmState | null>(null)

// lastConfirmReason keeps the submitted reason after the dialog closes so
// callers can read it after confirm() resolves (state is nulled on close).
const lastConfirmReason = ref('')

export function readConfirmReason(): string {
  return lastConfirmReason.value
}

export function useAdminConfirm(): AdminConfirmPort {
  function confirm(options: AdminConfirmOptions): Promise<boolean> {
    return new Promise<boolean>((resolve, reject) => {
      lastConfirmReason.value = ''
      activeAdminConfirm.value = {
        open: true,
        title: options.title,
        message: options.message,
        confirmLabel: options.confirmLabel,
        intent: options.intent,
        reason: '',
        reasonEnabled: options.reason !== undefined,
        reasonRequired: options.reason?.required === true,
        reasonLabel: options.reason?.label ?? '操作原因（内部审计记录）',
        reasonPlaceholder: options.reason?.placeholder ?? '请填写操作原因…',
        loading: false,
        confirm: async () => {
          if (!activeAdminConfirm.value) return
          if (activeAdminConfirm.value.reasonRequired && !activeAdminConfirm.value.reason.trim()) {
            return
          }
          activeAdminConfirm.value.loading = true
          try {
            if (options.onConfirm) {
              await options.onConfirm()
            }
            lastConfirmReason.value = activeAdminConfirm.value?.reason.trim() ?? ''
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
        setReason: (value: string) => {
          if (activeAdminConfirm.value) {
            activeAdminConfirm.value.reason = value
          }
        },
      }
    })
  }

  return {
    confirm,
    readReason: readConfirmReason,
  }
}
