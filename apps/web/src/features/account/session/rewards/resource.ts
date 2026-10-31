import {
  listWorkspaceRewardRedemptions,
  redeemWorkspaceReward,
} from '../../../../api/generated-query/public/workspace-rewards/workspace-rewards'
import type {
  RedeemWorkspaceRewardResult,
  WorkspaceRewardRedemption,
  WorkspaceRewardRedemptionHistoryItem,
} from '../../../../api/generated-query/public/model'
import { buildContractRequestHeaders, newRequestId } from '@/api/http/contractKernel'
import { withSignal } from '../../../../core/utils/request'
import { workspaceRewardsRedeemIdempotencyKey } from '@/api/http/idempotencyKey'
import { i18n } from '../../../../core/i18n'
import {
  asWorkspaceBillingError,
  readContractListBody,
  requireAccountWorkspaceId,
  resolveWorkspaceBillingData,
} from '../workspaceBilling'

// User rewards boundary (W2 transports). Redemption and redemption
// history go through the Public Data Plane (`POST/GET
// /api/v1/workspaces/{id}/rewards/redemptions`, cursor pagination over
// the opaque `next_cursor`) with local view types; the deleted
// `/api/user/rewards/*` routes are gone. Redemption carries a
// header-only Idempotency-Key per attempt; key reuse with a different
// request renders 409 `reward_redeem_conflict`.
export type ListUserRewardRedemptionsParams = {
  cursor?: string
  page_size?: number
}

export type RewardRedemption = WorkspaceRewardRedemption
export type RewardRedemptionItem = WorkspaceRewardRedemptionHistoryItem

export type RewardRedemptionPageData = {
  redemptions: RewardRedemptionItem[]
  next_cursor?: string
}

export type RewardRedeemResult = RedeemWorkspaceRewardResult & {
  replayed: boolean
}

export const rewardRedemptionsQueryKeys = {
  all: ['workspace', 'rewards', 'redemptions'] as const,
  list: (workspaceId: string) => ['workspace', 'rewards', 'redemptions', workspaceId] as const,
}

export async function redeemRewardCode(
  workspaceId: string,
  code: string,
  options: { idempotencyKey?: string | undefined } = {},
): Promise<RewardRedeemResult> {
  const id = requireAccountWorkspaceId(workspaceId)
  const result = await resolveWorkspaceBillingData<RedeemWorkspaceRewardResult>(
    redeemWorkspaceReward(
      id,
      { data: { code: code.trim() } },
      {
        headers: buildContractRequestHeaders({
          requestId: newRequestId(),
          idempotencyKey: options.idempotencyKey ?? workspaceRewardsRedeemIdempotencyKey(),
        }),
      },
    ),
    i18n.global.t('account.errors.redeemCodeInvalid'),
  )
  return {
    ...result,
    replayed: result.replayed === true,
  }
}

export async function fetchRewardRedemptions(
  workspaceId: string,
  params: ListUserRewardRedemptionsParams,
  signal?: AbortSignal,
): Promise<RewardRedemptionPageData> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('account.errors.rewardsHistoryLoadFailed')
  // Paginated list bodies already carry the contract `{data, meta}` shape,
  // so they are read directly instead of unwrapped like `{data}` envelopes.
  let body: unknown
  try {
    body = await listWorkspaceRewardRedemptions(
      id,
      {
        ...(params.cursor === undefined ? {} : { cursor: params.cursor }),
        ...(params.page_size === undefined ? {} : { page_size: params.page_size }),
      },
      withSignal(signal),
    )
  } catch (error) {
    throw asWorkspaceBillingError(error, fallback)
  }
  const page = readContractListBody<{
    data: WorkspaceRewardRedemptionHistoryItem[]
    meta: { has_more: boolean; next_cursor?: string | null }
  }>(body, fallback)
  if (!Array.isArray(page?.data) || page?.meta == null) throw new Error(fallback)
  return {
    redemptions: page.data,
    ...(page.meta.has_more && page.meta.next_cursor ? { next_cursor: page.meta.next_cursor } : {}),
  }
}
