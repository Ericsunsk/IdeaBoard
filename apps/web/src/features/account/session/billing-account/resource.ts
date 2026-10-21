import { getWorkspaceBillingAccount } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceBillingAccount } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// Stable Billing Account boundary (issue #922).
//
// A Billing Account (ba_) holds the unified balance; many Workspaces may
// share one account (organization pool or explicit no-org link) while
// independent accounts stay isolated. The Workspace is the consumption
// location, the User is the real operator; the account identity never
// derives from the current Workspace Owner wallet and never rewrites
// history on member exit or owner change.
export type BillingAccountScope = {
  billingAccountId: string
  workspaceId: string
  isShared: boolean
  consumptionScope: WorkspaceBillingAccount['consumption_scope']
}

export const billingAccountQueryKeys = {
  all: ['workspace', 'billing-account'] as const,
  detail: (workspaceId: string, identity = '') =>
    ['workspace', 'billing-account', 'detail', workspaceId, identity] as const,
}

export async function fetchBillingAccount(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<BillingAccountScope> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('account.errors.billingAccountLoadFailed')
  const account = await resolveWorkspaceBillingData<WorkspaceBillingAccount>(
    getWorkspaceBillingAccount(id, { signal }),
    fallback,
  )
  if (
    account.workspace_id !== id ||
    !['account', 'workspace', 'member'].includes(account.consumption_scope)
  ) {
    throw new Error(fallback)
  }
  return {
    billingAccountId: account.billing_account_id,
    workspaceId: account.workspace_id,
    isShared: account.shared === true,
    consumptionScope: account.consumption_scope,
  }
}
