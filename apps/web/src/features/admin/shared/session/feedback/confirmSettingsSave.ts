import type { AdminConfirmPort } from './useAdminConfirm'

export async function confirmSettingsSave(
  confirm: AdminConfirmPort,
  options: {
    title: string
    message: string
    confirmLabel?: string | undefined
    risk?: 'reversible' | 'destructive' | undefined
  },
): Promise<boolean> {
  return confirm.confirm({
    title: options.title,
    message: options.message,
    confirmLabel: options.confirmLabel ?? '保存',
    intent: options.risk === 'destructive' ? 'danger' : 'warn',
  })
}
