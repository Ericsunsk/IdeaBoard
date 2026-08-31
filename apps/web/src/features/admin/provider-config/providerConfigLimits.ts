import type { AdminProviderAccountLimits } from './providerConfigTypes'

export const FALLBACK_ADMIN_ACCOUNT_LIMITS: AdminProviderAccountLimits = {
  maxConcurrent: { min: 1, max: 64, default: 3 },
  timeoutSeconds: { min: 1, max: 900, default: 120 },
}
