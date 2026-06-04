import {
  createAdminAccount,
  deleteAdminAccount,
  getAdminAccountLimits,
  listAdminAccounts,
  startAdminAccount,
  stopAdminAccount,
  updateAdminAccount,
} from '../../../../shared/api/generated-query/admin-accounts/admin-accounts'
import type { AccountLimitRange, AccountLimits } from '../../../../shared/api/generated-query/model'

export type { AdminAccount } from '../../../../shared/api/generated-query/model'
export type AdminAccountLimits = AccountLimits

type AdminAccountLimitRange = AccountLimitRange

export const FALLBACK_ADMIN_ACCOUNT_LIMITS: AdminAccountLimits = {
  max_concurrent: { min: 1, max: 64, default: 3 },
  timeout_seconds: { min: 1, max: 600, default: 120 },
}

function normalizeAdminAccountLimitRange(
  range: AdminAccountLimitRange | undefined,
  fallback: AdminAccountLimitRange,
): AdminAccountLimitRange {
  const rawMin = range?.min
  const rawMax = range?.max
  const rawDefault = range?.default
  const min =
    typeof rawMin === 'number' && Number.isFinite(rawMin) && rawMin > 0 ? rawMin : fallback.min
  const max =
    typeof rawMax === 'number' && Number.isFinite(rawMax) && rawMax >= min ? rawMax : fallback.max
  const defaultValue =
    typeof rawDefault === 'number' &&
    Number.isFinite(rawDefault) &&
    rawDefault >= min &&
    rawDefault <= max
      ? rawDefault
      : fallback.default
  return { min, max, default: defaultValue }
}

export function normalizeAdminAccountLimits(limits?: AdminAccountLimits): AdminAccountLimits {
  return {
    max_concurrent: normalizeAdminAccountLimitRange(
      limits?.max_concurrent,
      FALLBACK_ADMIN_ACCOUNT_LIMITS.max_concurrent,
    ),
    timeout_seconds: normalizeAdminAccountLimitRange(
      limits?.timeout_seconds,
      FALLBACK_ADMIN_ACCOUNT_LIMITS.timeout_seconds,
    ),
  }
}

export const adminAccountApi = {
  create: createAdminAccount,
  update: updateAdminAccount,
  delete: deleteAdminAccount,
  list: listAdminAccounts,
  start: startAdminAccount,
  stop: stopAdminAccount,
  getLimits: getAdminAccountLimits,
}

export default adminAccountApi
