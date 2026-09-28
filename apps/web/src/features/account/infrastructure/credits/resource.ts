import { getWorkspaceCreditsBalance } from '../../../../shared/api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceCreditsBalance } from '../../../../shared/api/generated-query/public/model'
import { i18n } from '../../../../shared/i18n'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// User credit wallet boundary (W2 transports). Reads go through the Public
// Data Plane (`GET /api/v1/workspaces/{id}/credits/balance`, transitional
// projection of the owner's wallet) with a local view type; the deleted
// `GET /api/user/credits/wallet` route is gone.
export type CreditWallet = {
  public_id: string
  display_name: string
  available_credits: number
  reserved_credits: number
}

export type CreditWalletSummary = CreditWallet

export const creditsQueryKeys = {
  all: ['workspace', 'credits'] as const,
  wallet: (workspaceId: string) => ['workspace', 'credits', 'wallet', workspaceId] as const,
}

export async function fetchCreditWallet(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<CreditWallet> {
  const id = requireAccountWorkspaceId(workspaceId)
  const balance = await resolveWorkspaceBillingData<WorkspaceCreditsBalance>(
    getWorkspaceCreditsBalance(id, { signal }),
    i18n.global.t('account.errors.walletLoadFailed'),
  )
  return {
    public_id: balance.workspace_id,
    display_name: '',
    available_credits: balance.available_credits,
    reserved_credits: balance.reserved_credits,
  }
}
