import {
  getControlRewardRedemption,
  listControlRewardRedemptions,
} from '../../../../../api/generated-query/control/control-credits/control-credits'
import { requireContractData } from '../../../../../api/contractKernel'
import { createControlError } from '../../../shared'
import type {
  ControlAdminRedemption,
  ControlAdminRedemptionDetail,
  ControlAdminRedemptionPage,
} from '../../../../../api/generated-query/control/model'

// Admin reward redemption boundary (W2 transports). List/get ride the
// Control Plane redemption operations
// (`/api/control/v1/rewards/codes/:id/redemptions*`) with the unified
// contract envelope (`{ data }` success / `{ error: { code, message,
// request_id } }` errors). The deleted
// `/api/admin/rewards/codes/:id/redemptions*` routes are gone; this
// module is the single transport. View types and normalizers keep their
// shapes so the UI and specs keep working.
export type RewardCodeRedemption = {
  id: number
  rewardCodeId: number
  userPublicId: string
  displayName: string
  creditedAmount: number
  redeemedAt: string
}

export type RewardCodeRedemptionDetail = RewardCodeRedemption & {
  creditTransactionId: string
  maskedCodeHint: string
}

type RewardCodeRedemptionPage = {
  redemptions: RewardCodeRedemption[]
  nextBeforeId?: number | undefined
}

export type RewardCodeRedemptionResource = {
  list: (
    rewardCodeId: number,
    input?: {
      beforeId?: number | undefined
      limit?: number | undefined
      signal?: AbortSignal | undefined
    },
  ) => Promise<RewardCodeRedemptionPage>
  get: (
    rewardCodeId: number,
    redemptionId: number,
    input?: { signal?: AbortSignal },
  ) => Promise<RewardCodeRedemptionDetail>
}

export function createRewardCodeRedemptionResource(): RewardCodeRedemptionResource {
  return {
    list: listRewardCodeRedemptions,
    get: getRewardCodeRedemption,
  }
}

async function listRewardCodeRedemptions(
  rewardCodeId: number,
  input: {
    beforeId?: number | undefined
    limit?: number | undefined
    signal?: AbortSignal | undefined
  } = {},
): Promise<RewardCodeRedemptionPage> {
  const params: { before_id?: number; limit?: number } = {}
  if (input.beforeId != null) params.before_id = input.beforeId
  if (input.limit != null) params.limit = input.limit
  const body = await listControlRewardRedemptions(rewardCodeId, params, {
    ...(input.signal == null ? {} : { signal: input.signal }),
  })
  const page = requireContractData<ControlAdminRedemptionPage>(body, '加载兑换记录失败')
  if (!Array.isArray(page.redemptions)) {
    throw createControlError('control_failed', '加载兑换记录失败')
  }
  const result: RewardCodeRedemptionPage = {
    redemptions: page.redemptions.map(normalizeRedemption),
  }
  if (page.next_before_id != null) result.nextBeforeId = page.next_before_id
  return result
}

async function getRewardCodeRedemption(
  rewardCodeId: number,
  redemptionId: number,
  input: { signal?: AbortSignal } = {},
): Promise<RewardCodeRedemptionDetail> {
  const body = await getControlRewardRedemption(rewardCodeId, redemptionId, {
    ...(input.signal == null ? {} : { signal: input.signal }),
  })
  const detail = requireContractData<ControlAdminRedemptionDetail>(body, '加载兑换详情失败')
  return {
    ...normalizeRedemption(detail),
    creditTransactionId: detail.credit_transaction_id,
    maskedCodeHint: detail.masked_code_hint,
  }
}

function normalizeRedemption(item: ControlAdminRedemption): RewardCodeRedemption {
  return {
    id: item.id,
    rewardCodeId: item.reward_code_id,
    userPublicId: item.user_public_id,
    displayName: item.display_name,
    creditedAmount: item.credited_amount,
    redeemedAt: item.redeemed_at,
  }
}
