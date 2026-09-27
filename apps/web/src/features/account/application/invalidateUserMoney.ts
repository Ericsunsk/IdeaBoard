// Single money-invalidation channel for user Idea reads.
//
// Credits (wallet balance) and billing records (ledger read model) always
// move together: every grant/capture/release/settlement path must refresh
// both. Do NOT invalidate `creditsQueryKeys` / `billingRecordsQueryKeys`
// ad hoc — route through here so the pair can never drift apart.
//
// Reward redemption history (`rewardRedemptionsQueryKeys`) intentionally
// stays at the redemption site: only redemption changes that history, so
// payment/settlement flows must not stale-mark it.
import type { QueryClient } from '@tanstack/vue-query'
import { billingRecordsQueryKeys, creditsQueryKeys } from '../infrastructure'

export function invalidateUserMoney(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: creditsQueryKeys.all })
  void queryClient.invalidateQueries({ queryKey: billingRecordsQueryKeys.all })
}
