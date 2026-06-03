import {
  createAdminAccount,
  deleteAdminAccount,
  getAdminAccountLimits,
  listAdminAccounts,
  startAdminAccount,
  stopAdminAccount,
  updateAdminAccount,
} from './generated-query/admin-accounts/admin-accounts'
import type { AccountLimitRange, AccountLimits, AdminAccount } from './generated-query/model'

export type ApiAccount = AdminAccount
export type { AccountLimitRange, AccountLimits }

export const FALLBACK_ACCOUNT_LIMITS: AccountLimits = {
  max_concurrent: { min: 1, max: 64, default: 3 },
  timeout_seconds: { min: 1, max: 600, default: 120 },
}

function normalizeLimitRange(
  range: AccountLimitRange | undefined,
  fallback: AccountLimitRange,
): AccountLimitRange {
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

export function normalizeAccountLimits(limits?: AccountLimits): AccountLimits {
  return {
    max_concurrent: normalizeLimitRange(
      limits?.max_concurrent,
      FALLBACK_ACCOUNT_LIMITS.max_concurrent,
    ),
    timeout_seconds: normalizeLimitRange(
      limits?.timeout_seconds,
      FALLBACK_ACCOUNT_LIMITS.timeout_seconds,
    ),
  }
}

export const createAccount = createAdminAccount

export const updateAccount = updateAdminAccount

export const deleteAccount = deleteAdminAccount

export const listAccounts = listAdminAccounts

export const startAccount = startAdminAccount

export const stopAccount = stopAdminAccount

export const getAccountLimits = getAdminAccountLimits

export const accountApi = {
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
  getAccountLimits,
}

export default accountApi
