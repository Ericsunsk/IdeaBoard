import type {
  AdminConfirmPort,
  AdminConfirmReasonSpec,
} from '../composables/useAdminConfirm'
import { readConfirmReason } from '../composables/useAdminConfirm'
import { useAdminToast } from './toast'

// Operator-friction tiers for admin confirmations. Backend already treats
// connectivity probes as POST read-guards (no reason ceremony), while every
// real mutation still requires a non-empty operator reason, so the tiers
// below only change how much typing the dialog demands — never whether the
// transport carries a reason.
export type AdminConfirmRisk = 'probe' | 'reversible' | 'destructive'

// Probes skip the dialog entirely; the fixed note keeps the audit record
// informative without operator typing.
export const ADMIN_PROBE_OPERATOR_REASON = 'connectivity probe'

// Reversible writes keep the dialog but the note is optional: an empty
// field falls back to this default so one click still proceeds.
export const ADMIN_REVERSIBLE_DEFAULT_REASON = '常规管理操作'

export const ADMIN_REASON_REQUIRED_MESSAGE = '需要填写操作原因'

// Control Plane mutations require an operator reason (audit). Settings
// saves collect it through the shared confirm dialog. Returns the trimmed
// reason, or null when the operator cancels (reversible empty notes fall
// back to the default so they proceed; destructive empty notes toast and
// abort instead of vanishing silently).
export async function confirmSettingsSave(
  confirm: AdminConfirmPort,
  options: {
    title: string
    message: string
    confirmLabel?: string | undefined
    reason?: AdminConfirmReasonSpec | undefined
    risk?: AdminConfirmRisk | undefined
  },
): Promise<string | null> {
  const risk = options.risk ?? 'reversible'
  if (risk === 'probe') {
    return ADMIN_PROBE_OPERATOR_REASON
  }
  if (risk === 'destructive') {
    const confirmed = await confirm.confirm({
      title: options.title,
      message: options.message,
      confirmLabel: options.confirmLabel ?? '确认',
      intent: 'danger',
      reason: options.reason ?? { required: true },
    })
    if (!confirmed) return null
    return requireDestructiveReason(confirm)
  }
  const confirmed = await confirm.confirm({
    title: options.title,
    message: options.message,
    confirmLabel: options.confirmLabel ?? '保存',
    intent: 'warn',
    reason: options.reason ?? { required: false, placeholder: '可留空，将记录为常规操作…' },
  })
  if (!confirmed) return null
  return readReversibleReason(confirm)
}

// Direct-confirm destructive call sites (delete, ban, role change, refund,
// revoke, unban, impersonate) keep their own dialog shape and call this to
// turn the trimmed reason into null-with-toast on empty instead of a silent
// abort.
export function requireDestructiveReason(confirm: AdminConfirmPort): string | null {
  const reason = (confirm.readReason?.() ?? readConfirmReason()).trim()
  if (reason === '') {
    useAdminToast().error(ADMIN_REASON_REQUIRED_MESSAGE)
    return null
  }
  return reason
}

// Direct-confirm reversible call sites (enable/disable toggles): empty notes
// fall back to the default so one click proceeds.
export function readReversibleReason(confirm: AdminConfirmPort): string {
  const reason = (confirm.readReason?.() ?? readConfirmReason()).trim()
  return reason === '' ? ADMIN_REVERSIBLE_DEFAULT_REASON : reason
}
