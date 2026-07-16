import { listAdminRewardRedemptions } from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  AdminRewardRedemption as GeneratedAdminRewardRedemption,
  AdminRewardRedemptionPage as GeneratedAdminRewardRedemptionPage,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'

export type AdminRewardRedemption = {
  id: number
  userId: number
  creditAmount: number
  maskedCodeHint: string
  creditTransactionId: string
  redeemedAt: string
}

export type AdminRewardRedemptionPage = {
  redemptions: AdminRewardRedemption[]
  nextBeforeId?: number
}

export type AdminRewardRedemptionResource = {
  list: (campaignId: number, beforeId?: number) => Promise<AdminRewardRedemptionPage>
}

export function createAdminRewardRedemptionResource(): AdminRewardRedemptionResource {
  return {
    list: async (campaignId, beforeId) =>
      toAdminRewardRedemptionPage(
        await resolveApiData(
          listAdminRewardRedemptions(campaignId, { before_id: beforeId, limit: 20 }),
          '加载兑换记录失败',
        ),
      ),
  }
}

function toAdminRewardRedemption(
  redemption: GeneratedAdminRewardRedemption,
): AdminRewardRedemption {
  return {
    id: redemption.id,
    userId: redemption.user_id,
    creditAmount: redemption.credit_amount,
    maskedCodeHint: redemption.masked_code_hint,
    creditTransactionId: redemption.credit_transaction_id,
    redeemedAt: redemption.redeemed_at,
  }
}

function toAdminRewardRedemptionPage(
  page: GeneratedAdminRewardRedemptionPage,
): AdminRewardRedemptionPage {
  return {
    redemptions: page.redemptions.map(toAdminRewardRedemption),
    nextBeforeId: page.next_before_id,
  }
}
