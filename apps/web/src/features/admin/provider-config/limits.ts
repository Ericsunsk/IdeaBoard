import type { AdminProviderAccountLimits } from './types'

export const FALLBACK_ADMIN_ACCOUNT_LIMITS: AdminProviderAccountLimits = {
  maxConcurrent: { min: 1, max: 64, default: 3 },
  timeoutSeconds: { min: 1, max: 900, default: 120 },
}
