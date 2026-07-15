import {
  activateAdminRewardCampaign,
  cancelAdminRewardCampaign,
  createAdminRewardCampaign,
  deleteAdminRewardCampaign,
  issueAdminRewardPublicCode,
  listAdminRewardCampaigns,
  pauseAdminRewardCampaign,
  resumeAdminRewardCampaign,
  updateAdminRewardCampaign,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  IssuedRewardPublicCode as GeneratedIssuedRewardPublicCode,
  RewardCampaign as GeneratedRewardCampaign,
  RewardCampaignPage as GeneratedRewardCampaignPage,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../../../shared/api/apiResponse'
import { shanghaiDateTimeInputToISO } from './rewardCampaignTime'

type RewardCampaignSettingsInput = {
  displayName: string
  internalNote: string
  creditAmount: number
  maxRedemptions: number
  startsAt: string
  expiresAt: string
  reason: string
}

export type CreateRewardCampaignInput = RewardCampaignSettingsInput & {
  mode: GeneratedRewardCampaign['mode']
}

export type UpdateRewardCampaignInput = RewardCampaignSettingsInput

export type RewardCampaign = {
  id: number
  displayName: string
  internalNote: string
  creditAmount: number
  mode: GeneratedRewardCampaign['mode']
  lifecycleStatus: GeneratedRewardCampaign['lifecycle_status']
  status: GeneratedRewardCampaign['status']
  maxRedemptions: number
  redemptionCount: number
  issuedCodeCount: number
  validUnusedCodeCount: number
  startsAt?: string
  expiresAt?: string
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

export type RewardCampaignConflictKind = 'rules_frozen' | 'draft_not_empty'

export class RewardCampaignConflictError extends Error {
  readonly kind: RewardCampaignConflictKind

  constructor(kind: RewardCampaignConflictKind, options?: ErrorOptions) {
    super(kind === 'rules_frozen' ? 'reward campaign rules are frozen' : 'reward draft is not empty', options)
    this.name = 'RewardCampaignConflictError'
    this.kind = kind
  }
}

export type AdminRewardCampaignResource = {
  list: (beforeId?: number) => Promise<RewardCampaignPage>
  create: (input: CreateRewardCampaignInput) => Promise<RewardCampaign>
  update: (campaignId: number, input: UpdateRewardCampaignInput) => Promise<RewardCampaign>
  issuePublicCode: (
    campaignId: number,
    input: { code: string; reason: string },
  ) => Promise<IssuedPublicCode>
  activate: (campaignId: number, reason: string) => Promise<RewardCampaign>
  pause: (campaignId: number, reason: string) => Promise<RewardCampaign>
  resume: (campaignId: number, reason: string) => Promise<RewardCampaign>
  cancel: (campaignId: number, reason: string) => Promise<RewardCampaign>
  delete: (campaignId: number, reason: string) => Promise<void>
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
            mode: input.mode,
            max_redemptions: input.maxRedemptions,
            ...optionalSchedulePayload(input),
            reason: input.reason.trim(),
          }),
          '创建奖励活动失败',
        ),
      ),
    update: async (campaignId, input) =>
      toCampaign(
        await resolveApiData(
          mapCampaignConflict(
            updateAdminRewardCampaign(campaignId, {
              display_name: input.displayName.trim(),
              internal_note: input.internalNote.trim(),
              credit_amount: input.creditAmount,
              max_redemptions: input.maxRedemptions,
              starts_at: shanghaiDateTimeInputToISO(input.startsAt) ?? null,
              expires_at: shanghaiDateTimeInputToISO(input.expiresAt) ?? null,
              reason: input.reason.trim(),
            }),
            'rules_frozen',
          ),
          '更新奖励活动失败',
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
    pause: async (campaignId, reason) =>
      toCampaign(
        await resolveApiData(
          pauseAdminRewardCampaign(campaignId, { reason: reason.trim() }),
          '暂停奖励活动失败',
        ),
      ),
    resume: async (campaignId, reason) =>
      toCampaign(
        await resolveApiData(
          resumeAdminRewardCampaign(campaignId, { reason: reason.trim() }),
          '恢复奖励活动失败',
        ),
      ),
    cancel: async (campaignId, reason) =>
      toCampaign(
        await resolveApiData(
          cancelAdminRewardCampaign(campaignId, { confirmed: true, reason: reason.trim() }),
          '取消奖励活动失败',
        ),
      ),
    delete: async (campaignId, reason) =>
      resolveApiOk(
        mapCampaignConflict(
          deleteAdminRewardCampaign(campaignId, { reason: reason.trim() }),
          'draft_not_empty',
        ),
        '删除奖励活动失败',
      ),
  }
}

function toCampaign(campaign: GeneratedRewardCampaign): RewardCampaign {
  return {
    id: campaign.id,
    displayName: campaign.display_name,
    internalNote: campaign.internal_note,
    creditAmount: campaign.credit_amount,
    mode: campaign.mode,
    lifecycleStatus: campaign.lifecycle_status,
    status: campaign.status,
    maxRedemptions: campaign.max_redemptions,
    redemptionCount: campaign.redemption_count,
    issuedCodeCount: campaign.issued_code_count,
    validUnusedCodeCount: campaign.valid_unused_code_count,
    startsAt: campaign.starts_at,
    expiresAt: campaign.expires_at,
    createdBy: campaign.created_by,
    createdAt: campaign.created_at,
    updatedAt: campaign.updated_at,
  }
}

function optionalSchedulePayload(input: CreateRewardCampaignInput) {
  const startsAt = shanghaiDateTimeInputToISO(input.startsAt)
  const expiresAt = shanghaiDateTimeInputToISO(input.expiresAt)
  return {
    ...(startsAt ? { starts_at: startsAt } : {}),
    ...(expiresAt ? { expires_at: expiresAt } : {}),
  }
}

async function mapCampaignConflict<T>(
  request: Promise<T>,
  kind: RewardCampaignConflictKind,
): Promise<T> {
  try {
    const response = await request
    if (isConflictResponse(response)) throw new RewardCampaignConflictError(kind)
    return response
  } catch (error) {
    if (error instanceof RewardCampaignConflictError) throw error
    if (isConflictResponse(error)) {
      throw new RewardCampaignConflictError(kind, { cause: error })
    }
    throw error
  }
}

function isConflictResponse(value: unknown): boolean {
  if (!value || typeof value !== 'object') return false
  const response = value as {
    code?: unknown
    status?: unknown
    data?: { code?: unknown }
    response?: { status?: unknown }
  }
  return (
    Number(response.code) === 409 ||
    Number(response.status) === 409 ||
    Number(response.data?.code) === 409 ||
    Number(response.response?.status) === 409
  )
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
