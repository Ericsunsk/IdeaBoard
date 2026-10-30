import { getWorkspaceBootstrap } from '../../../api/generated-query/public/workspaces/workspaces'
import type { WorkspaceBootstrap } from '../../../api/generated-query/public/model'
import { queryClient } from '../../../core/query/queryClient'
import {
  billingAccountQueryKeys,
  type BillingAccountScope,
  creditsQueryKeys,
  type CreditWallet,
} from '../../account'
import { writeSiteBrandingQueryCache } from '../../../core/composables/useSiteBranding'

/**
 * Fetches consolidated workspace bootstrap data (workspace, billing account,
 * wallet, branding) in 1 RTT and seeds TanStack Query cache.
 */
export async function fetchWorkspaceBootstrap(
  workspaceId: string,
  identity: string,
  signal?: AbortSignal,
): Promise<WorkspaceBootstrap> {
  const ws = workspaceId.trim()
  if (!ws) throw new Error('workspaceId is required')

  const envelope = await getWorkspaceBootstrap(ws, { signal })
  const data = envelope?.data
  if (!data?.workspace || !data?.billing_account || !data?.wallet) {
    throw new Error('Workspace bootstrap payload incomplete')
  }

  // 1. Seed Billing Account Cache
  const billingAccountScope: BillingAccountScope = {
    billingAccountId: data.billing_account.billing_account_id,
    workspaceId: data.billing_account.workspace_id,
    isShared: data.billing_account.shared === true,
    consumptionScope: data.billing_account.consumption_scope,
    canManageMemberQuotas: data.billing_account.can_manage_member_quotas,
  }
  queryClient.setQueryData(billingAccountQueryKeys.detail(ws, identity), {
    ...billingAccountScope,
    identity,
  })

  // 2. Seed Credit Wallet Cache
  const wallet: CreditWallet = {
    billing_account_id: data.wallet.billing_account_id,
    public_id: data.wallet.workspace_id,
    display_name: '',
    available_credits: data.wallet.available_credits,
    reserved_credits: data.wallet.reserved_credits,
  }
  queryClient.setQueryData(
    creditsQueryKeys.wallet(ws, data.billing_account.billing_account_id, identity),
    wallet,
  )

  // 3. Seed Site Branding Cache
  if (data.branding) {
    writeSiteBrandingQueryCache(queryClient, data.branding)
  }

  return data
}
