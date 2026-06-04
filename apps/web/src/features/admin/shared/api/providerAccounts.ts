import {
  createAdminAccount,
  deleteAdminAccount,
  getAdminAccountLimits,
  listAdminAccounts,
  startAdminAccount,
  stopAdminAccount,
  updateAdminAccount,
} from '../../../../shared/api/generated-query/admin-accounts/admin-accounts'
import type {
  AccountLimitRange,
  AccountLimits,
  AdminAccount,
} from '../../../../shared/api/generated-query/model'

export type ProviderAccount = AdminAccount
export type ProviderAccountLimits = AccountLimits

type ProviderAccountLimitRange = AccountLimitRange

export const FALLBACK_PROVIDER_ACCOUNT_LIMITS: ProviderAccountLimits = {
  max_concurrent: { min: 1, max: 64, default: 3 },
  timeout_seconds: { min: 1, max: 600, default: 120 },
}

function normalizeProviderAccountLimitRange(
  range: ProviderAccountLimitRange | undefined,
  fallback: ProviderAccountLimitRange,
): ProviderAccountLimitRange {
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

export function normalizeProviderAccountLimits(
  limits?: ProviderAccountLimits,
): ProviderAccountLimits {
  return {
    max_concurrent: normalizeProviderAccountLimitRange(
      limits?.max_concurrent,
      FALLBACK_PROVIDER_ACCOUNT_LIMITS.max_concurrent,
    ),
    timeout_seconds: normalizeProviderAccountLimitRange(
      limits?.timeout_seconds,
      FALLBACK_PROVIDER_ACCOUNT_LIMITS.timeout_seconds,
    ),
  }
}

export const providerAccountApi = {
  createProviderAccount: createAdminAccount,
  updateProviderAccount: updateAdminAccount,
  deleteProviderAccount: deleteAdminAccount,
  listProviderAccounts: listAdminAccounts,
  startProviderAccount: startAdminAccount,
  stopProviderAccount: stopAdminAccount,
  getProviderAccountLimits: getAdminAccountLimits,
}

export default providerAccountApi
