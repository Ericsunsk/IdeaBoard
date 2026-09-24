import type {
  AdminConfirmPort,
  AdminConfirmReasonSpec,
} from '../composables/useAdminConfirm'
import { readConfirmReason } from '../composables/useAdminConfirm'

// Control Plane mutations require an operator reason (audit). Settings
// saves collect it through the shared confirm dialog so every high-risk
// settings write carries an explicit operator note. Returns the trimmed
// reason, or null when the operator cancels.
export async function confirmSettingsSave(
  confirm: AdminConfirmPort,
  options: {
    title: string
    message: string
    confirmLabel?: string | undefined
    reason?: AdminConfirmReasonSpec | undefined
  },
): Promise<string | null> {
  const confirmed = await confirm.confirm({
    title: options.title,
    message: options.message,
    confirmLabel: options.confirmLabel ?? '保存',
    intent: 'warn',
    reason: options.reason ?? { required: true },
  })
  if (!confirmed) return null
  const reason = (confirm.readReason?.() ?? readConfirmReason()).trim()
  return reason === '' ? null : reason
}
