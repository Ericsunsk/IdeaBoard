import {
  getControlRewardRedemption,
  listControlRewardRedemptions,
} from '../../../../shared/api/generated-query/control/control-credits/control-credits'
import type {
  ControlAdminRedemption,
  ControlAdminRedemptionDetail,
  ControlAdminRedemptionPage,
} from '../../../../shared/api/generated-query/control/model'

// Admin reward redemption boundary (W2 transports). List/get ride the
// Control Plane redemption operations
// (`/api/control/v1/rewards/codes/:id/redemptions*`, guarded proxies
// keeping the legacy `{code, msg, data}` envelope). The deleted
// `/api/admin/rewards/codes/:id/redemptions*` routes are gone; this
// module is the single transport. View types and normalizers keep their
// shapes so the UI and specs keep working.
export type GeneratedRedemption = {
  id: number
  reward_code_id: number
  user_public_id: string
  display_name: string
  credited_amount: number
  redeemed_at: string
}

export type GeneratedRedemptionDetail = GeneratedRedemption & {
  credit_transaction_id: string
  masked_code_hint: string
}

export type GeneratedRedemptionPage = {
  redemptions: GeneratedRedemption[]
  next_before_id?: number
}

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

export type RewardCodeRedemptionPage = {
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
  const page = readLegacyData<ControlAdminRedemptionPage>(body, '加载兑换记录失败')
  if (!Array.isArray(page.redemptions)) throw controlError('control_failed', '加载兑换记录失败')
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
  const detail = readLegacyData<ControlAdminRedemptionDetail>(body, '加载兑换详情失败')
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

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// Control reward routes keep the legacy `{ code, msg, data? }` shape: a
// numeric `code` of 0 means success, anything else is a failure.
function readLegacyData<T>(body: unknown, fallback: string): T {
  if (isRecord(body) && 'code' in body) {
    if (body.code !== 0) {
      const message = typeof body.msg === 'string' && body.msg ? body.msg : fallback
      throw controlError(typeof body.code === 'string' ? body.code : 'control_failed', message)
    }
    if (body.data == null) throw controlError('control_failed', fallback)
    return body.data as T
  }
  throw controlError('control_failed', fallback)
}
