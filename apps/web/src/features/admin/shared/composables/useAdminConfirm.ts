import { useConfirm } from 'primevue/useconfirm'

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

export function useAdminConfirm(): AdminConfirmPort {
  const confirm = useConfirm()

  return {
    request(request) {
      confirm.require({
        header: request.title,
        message: request.message,
        rejectLabel: '取消',
        acceptLabel: request.confirmLabel,
        acceptProps: {
          severity: request.intent,
        },
        rejectProps: {
          severity: 'secondary',
          outlined: true,
        },
        accept: () => {
          void request.onConfirm()
        },
      })
    },
  }
}
