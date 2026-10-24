import { getWorkspaceCreditsBalance } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceCreditsBalance } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// User credit wallet boundary (v1 public plane).
//
// Reads go through `GET /api/v1/workspaces/{id}/credits/balance`, which
// reports the stable Billing Account (ba_) balance behind the Workspace:
// every Workspace sharing one account reports the same available and
// reserved Ideas, with or without an Organization; independent accounts
// stay isolated. There is no owner-wallet projection and no fallback.
export type CreditWallet = {
  billing_account_id: string
  public_id: string
  display_name: string
  available_credits: number
  reserved_credits: number
}

export type CreditWalletSummary = CreditWallet

export const creditsQueryKeys = {
  all: ['workspace', 'credits'] as const,
  // Scalar-only key: workspace + account + identity. Never embed scope
  // objects so key equality stays referential-stable across renders.
  wallet: (workspaceId: string, billingAccountId: string, identity: string) =>
    ['workspace', 'credits', 'wallet', workspaceId, billingAccountId, identity] as const,
}

export async function fetchCreditWallet(
  workspaceId: string,
  signal?: AbortSignal,
  expectedBillingAccountId?: string,
): Promise<CreditWallet> {
  const id = requireAccountWorkspaceId(workspaceId)
  const balance = await resolveWorkspaceBillingData<WorkspaceCreditsBalance>(
    getWorkspaceCreditsBalance(id, { signal }),
    i18n.global.t('account.errors.walletLoadFailed'),
  )
  const accountId = balance.billing_account_id?.trim() ?? ''
  if (!accountId) throw new Error(i18n.global.t('account.errors.walletLoadFailed'))
  // Late responses from a previous account must never populate the new
  // scope: the caller names the account it resolved, and a mismatch fails
  // closed instead of mixing balances across accounts.
  if (expectedBillingAccountId && accountId !== expectedBillingAccountId) {
    throw new Error(i18n.global.t('account.errors.walletLoadFailed'))
  }
  return {
    billing_account_id: accountId,
    public_id: balance.workspace_id,
    display_name: '',
    available_credits: balance.available_credits,
    reserved_credits: balance.reserved_credits,
  }
}
