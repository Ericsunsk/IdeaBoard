import {
  activateAdminRewardCampaign,
  createAdminRewardCampaign,
  issueAdminRewardPublicCode,
  listAdminRewardCampaigns,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  IssuedRewardPublicCode as GeneratedIssuedRewardPublicCode,
  RewardCampaign as GeneratedRewardCampaign,
  RewardCampaignPage as GeneratedRewardCampaignPage,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'

export type CreateRewardCampaignInput = {
  displayName: string
  internalNote: string
  creditAmount: number
  maxRedemptions: number
  reason: string
}

export type RewardCampaign = {
  id: number
  displayName: string
  internalNote: string
  creditAmount: number
  status: 'draft' | 'active'
  maxRedemptions: number
  redemptionCount: number
  createdBy: number
  createdAt: string
  updatedAt: string
}

export type RewardCampaignPage = {
  campaigns: RewardCampaign[]
  nextBeforeId?: number
}

export type IssuedPublicCode = {
  id: number
  campaignId: number
  hint: string
  plaintext: string
}

export type AdminRewardCampaignResource = {
  list: (beforeId?: number) => Promise<RewardCampaignPage>
  create: (input: CreateRewardCampaignInput) => Promise<RewardCampaign>
  issuePublicCode: (
    campaignId: number,
    input: { code: string; reason: string },
  ) => Promise<IssuedPublicCode>
  activate: (campaignId: number, reason: string) => Promise<RewardCampaign>
}

export function createAdminRewardCampaignResource(): AdminRewardCampaignResource {
  return {
    list: async (beforeId) =>
      toCampaignPage(
        await resolveApiData(
          listAdminRewardCampaigns({ before_id: beforeId, limit: 20 }),
          '加载奖励活动失败',
        ),
      ),
    create: async (input) =>
      toCampaign(
        await resolveApiData(
          createAdminRewardCampaign({
            display_name: input.displayName.trim(),
            internal_note: input.internalNote.trim(),
            credit_amount: input.creditAmount,
            max_redemptions: input.maxRedemptions,
            reason: input.reason.trim(),
          }),
          '创建奖励活动失败',
        ),
      ),
    issuePublicCode: async (campaignId, input) =>
      toIssuedPublicCode(
        await resolveApiData(
          issueAdminRewardPublicCode(campaignId, {
            code: input.code.trim(),
            reason: input.reason.trim(),
          }),
          '发行公共兑换码失败',
        ),
      ),
    activate: async (campaignId, reason) =>
      toCampaign(
        await resolveApiData(
          activateAdminRewardCampaign(campaignId, { reason: reason.trim() }),
          '激活奖励活动失败',
        ),
      ),
  }
}

function toCampaign(campaign: GeneratedRewardCampaign): RewardCampaign {
  return {
    id: campaign.id,
    displayName: campaign.display_name,
    internalNote: campaign.internal_note,
    creditAmount: campaign.credit_amount,
    status: campaign.status,
    maxRedemptions: campaign.max_redemptions,
    redemptionCount: campaign.redemption_count,
    createdBy: campaign.created_by,
    createdAt: campaign.created_at,
    updatedAt: campaign.updated_at,
  }
}

function toCampaignPage(page: GeneratedRewardCampaignPage): RewardCampaignPage {
  return {
    campaigns: page.campaigns.map(toCampaign),
    nextBeforeId: page.next_before_id,
  }
}

function toIssuedPublicCode(issued: GeneratedIssuedRewardPublicCode): IssuedPublicCode {
  return {
    id: issued.code.id,
    campaignId: issued.code.campaign_id,
    hint: issued.code.hint,
    plaintext: issued.plaintext,
  }
}
