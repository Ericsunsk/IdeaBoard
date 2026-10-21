// Single money-invalidation channel for user Idea reads.
//
// Credits (wallet balance), billing records (ledger read model), and usage
// (settled captures + task snapshots) always move together: every
// grant/capture/release/settlement path must refresh all three. Do NOT
// invalidate `creditsQueryKeys` / `billingRecordsQueryKeys` /
// `usageQueryKeys` ad hoc — route through here so the trio can never drift
// apart. The billing-account scope (`billingAccountQueryKeys`) is stable
// identity and needs no refresh on settlement.
//
// Reward redemption history (`rewardRedemptionsQueryKeys`) intentionally
// stays at the redemption site: only redemption changes that history, so
// payment/settlement flows must not stale-mark it.
import type { QueryClient } from '@tanstack/vue-query'
import { billingRecordsQueryKeys } from './billing/resource'
import { creditsQueryKeys } from './credits/resource'
import { usageQueryKeys } from './usage/resource'

export function invalidateUserMoney(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: creditsQueryKeys.all })
  void queryClient.invalidateQueries({ queryKey: billingRecordsQueryKeys.all })
  void queryClient.invalidateQueries({ queryKey: usageQueryKeys.all })
}
