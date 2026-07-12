export const MAX_ADMIN_ACTION_REASON_LENGTH = 1000

export interface AdminActionReasonValidation {
  normalizedReason: string
  error: string
  valid: boolean
}

export function validateAdminActionReason(reason: string): AdminActionReasonValidation {
  const normalizedReason = reason.trim()
  let error = ''

  if (!normalizedReason) {
    error = '请输入操作原因'
  } else if (Array.from(normalizedReason).length > MAX_ADMIN_ACTION_REASON_LENGTH) {
    error = `操作原因不能超过 ${MAX_ADMIN_ACTION_REASON_LENGTH} 个字符`
  }

  return {
    normalizedReason,
    error,
    valid: !error,
  }
}
