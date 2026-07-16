import {
  lookupAdminRewardCode,
  revokeAdminRewardCode,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type { AdminRewardCodeMetadata as GeneratedAdminRewardCodeMetadata } from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'

export type AdminRewardCode = {
  id: number
  campaignId: number
  campaignDisplayName: string
  batchId?: number
  maskedCodeHint: string
  status: GeneratedAdminRewardCodeMetadata['status']
  issuedAt: string
  revokedAt?: string
  redemptionId?: number
  redeemedAt?: string
}

export type AdminRewardCodeResource = {
  lookup: (code: string) => Promise<AdminRewardCode>
  revoke: (codeId: number, reason: string) => Promise<AdminRewardCode>
}

export function createAdminRewardCodeResource(): AdminRewardCodeResource {
  return {
    lookup: async (code) =>
      toAdminRewardCode(await resolveApiData(lookupAdminRewardCode({ code }), '查找兑换码失败')),
    revoke: async (codeId, reason) =>
      toAdminRewardCode(
        await resolveApiData(
          revokeAdminRewardCode(codeId, { confirmed: true, reason: reason.trim() }),
          '撤销兑换码失败',
        ),
      ),
  }
}

function toAdminRewardCode(code: GeneratedAdminRewardCodeMetadata): AdminRewardCode {
  return {
    id: code.id,
    campaignId: code.campaign_id,
    campaignDisplayName: code.campaign_display_name,
    batchId: code.batch_id,
    maskedCodeHint: code.masked_code_hint,
    status: code.status,
    issuedAt: code.issued_at,
    revokedAt: code.revoked_at,
    redemptionId: code.redemption_id,
    redeemedAt: code.redeemed_at,
  }
}
