import type { RewardCampaign } from './rewardCampaignResource'
import { formatShanghaiDateTime } from './rewardCampaignTime'

const statusLabels = {
  draft: '草稿',
  cancelled: '已取消',
  expired: '已过期',
  exhausted: '已耗尽',
  paused: '已暂停',
  scheduled: '待开始',
  active: '进行中',
} satisfies Record<RewardCampaign['status'], string>

const lifecycleLabels = {
  draft: '草稿',
  active: '已激活',
  paused: '已暂停',
  cancelled: '已取消',
} satisfies Record<RewardCampaign['lifecycleStatus'], string>

export function rewardCampaignStatusLabel(status: RewardCampaign['status']) {
  return statusLabels[status]
}

export function rewardCampaignStatusSeverity(
  status: RewardCampaign['status'],
): 'success' | 'info' | 'warn' | 'danger' | 'secondary' {
  switch (status) {
    case 'active':
      return 'success'
    case 'scheduled':
      return 'info'
    case 'paused':
    case 'exhausted':
      return 'warn'
    case 'cancelled':
    case 'expired':
      return 'danger'
    default:
      return 'secondary'
  }
}

export function createRewardCampaignView(campaign: RewardCampaign) {
  const hasRedemptionCap = campaign.maxRedemptions > 0
  const progressPercent = hasRedemptionCap
    ? Math.min(
        100,
        Math.max(0, Math.round((campaign.redemptionCount / campaign.maxRedemptions) * 100)),
      )
    : 0
  return {
    ...campaign,
    statusLabel: rewardCampaignStatusLabel(campaign.status),
    statusSeverity: rewardCampaignStatusSeverity(campaign.status),
    lifecycleLabel: lifecycleLabels[campaign.lifecycleStatus],
    modeLabel: campaign.mode === 'single_use' ? '单次码' : '公共码',
    scheduleLabel: `${formatShanghaiDateTime(campaign.startsAt)} - ${formatShanghaiDateTime(campaign.expiresAt)}`,
    maxRedemptionsLabel: hasRedemptionCap ? `${campaign.maxRedemptions} 次` : '无限',
    progressLabel: hasRedemptionCap
      ? `${campaign.redemptionCount} / ${campaign.maxRedemptions}`
      : `${campaign.redemptionCount} 次已兑换`,
    progressPercent,
    hasRedemptionCap,
  }
}
