import type {
  AdminRewardCampaignResource,
  RewardCampaign,
} from './rewardCampaignResource'

export type RewardCampaignAuditedAction =
  | 'issue'
  | 'activate'
  | 'pause'
  | 'resume'
  | 'cancel'
  | 'delete'

export type RewardCampaignActionDialog = {
  title: string
  message: string
  confirmLabel: string
  intent: 'warn' | 'danger'
}

export type RewardCampaignActionResult =
  | { type: 'issued'; plaintext: string }
  | { type: 'campaign'; campaign: RewardCampaign }
  | { type: 'deleted' }

export type RewardCampaignActionResource = Pick<
  AdminRewardCampaignResource,
  'issuePublicCode' | 'activate' | 'pause' | 'resume' | 'cancel' | 'delete'
>

type RewardCampaignActionContext = {
  resource: RewardCampaignActionResource
  campaign: RewardCampaign
  reason: string
  publicCode: string
}

type RewardCampaignActionDescriptor = {
  dialog: (campaignName: string) => RewardCampaignActionDialog
  successMessage: string
  errorFallback: string
  execute: (context: RewardCampaignActionContext) => Promise<RewardCampaignActionResult>
}

const rewardCampaignActionDescriptors = {
  issue: {
    dialog: (campaignName) => ({
      title: '确认发行公共兑换码',
      message: `发行后“${campaignName}”的核心规则将被冻结。`,
      confirmLabel: '确认发行',
      intent: 'warn',
    }),
    successMessage: '公共兑换码已发行',
    errorFallback: '发行公共兑换码失败',
    execute: async ({ resource, campaign, publicCode, reason }) => {
      const issued = await resource.issuePublicCode(campaign.id, { code: publicCode, reason })
      return { type: 'issued', plaintext: issued.plaintext }
    },
  },
  activate: {
    dialog: (campaignName) => ({
      title: '确认激活奖励活动',
      message: `将激活“${campaignName}”。`,
      confirmLabel: '确认激活',
      intent: 'warn',
    }),
    successMessage: '奖励活动已激活',
    errorFallback: '激活奖励活动失败',
    execute: async ({ resource, campaign, reason }) => ({
      type: 'campaign',
      campaign: await resource.activate(campaign.id, reason),
    }),
  },
  pause: {
    dialog: (campaignName) => ({
      title: '确认暂停奖励活动',
      message: `暂停“${campaignName}”后，新的兑换请求将被拒绝。`,
      confirmLabel: '确认暂停',
      intent: 'warn',
    }),
    successMessage: '奖励活动已暂停',
    errorFallback: '暂停奖励活动失败',
    execute: async ({ resource, campaign, reason }) => ({
      type: 'campaign',
      campaign: await resource.pause(campaign.id, reason),
    }),
  },
  resume: {
    dialog: (campaignName) => ({
      title: '确认恢复奖励活动',
      message: `将恢复“${campaignName}”的兑换能力。`,
      confirmLabel: '确认恢复',
      intent: 'warn',
    }),
    successMessage: '奖励活动已恢复',
    errorFallback: '恢复奖励活动失败',
    execute: async ({ resource, campaign, reason }) => ({
      type: 'campaign',
      campaign: await resource.resume(campaign.id, reason),
    }),
  },
  cancel: {
    dialog: (campaignName) => ({
      title: '确认取消奖励活动',
      message: `取消“${campaignName}”不可恢复，所有未兑换 Code 将被撤销。`,
      confirmLabel: '确认取消',
      intent: 'danger',
    }),
    successMessage: '奖励活动已取消',
    errorFallback: '取消奖励活动失败',
    execute: async ({ resource, campaign, reason }) => ({
      type: 'campaign',
      campaign: await resource.cancel(campaign.id, reason),
    }),
  },
  delete: {
    dialog: (campaignName) => ({
      title: '确认删除空 Draft',
      message: `仅未发行 Code 的空 Draft 可删除。将删除“${campaignName}”。`,
      confirmLabel: '确认删除',
      intent: 'danger',
    }),
    successMessage: '空 Draft 已删除',
    errorFallback: '删除奖励活动失败',
    execute: async ({ resource, campaign, reason }) => {
      await resource.delete(campaign.id, reason)
      return { type: 'deleted' }
    },
  },
} satisfies Record<RewardCampaignAuditedAction, RewardCampaignActionDescriptor>

export function getRewardCampaignActionDescriptor(action: RewardCampaignAuditedAction) {
  return rewardCampaignActionDescriptors[action]
}
