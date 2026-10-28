import { defineFeedback } from '../../../shared'

export type RewardCodesFeedback = {
  loadFailed: (error: unknown) => void
  enabledToggled: (enabled: boolean) => void
  toggleFailed: (error: unknown, enabled: boolean) => void
  deleted: () => void
  deleteFailed: (error: unknown) => void
  saveFailed: (error: unknown, isEditing: boolean) => void
  redemptionsLoadFailed: (error: unknown) => void
  redemptionDetailFailed: (error: unknown) => void
}

const REWARD_CODES_COPY = {
  loadFailed: { error: '兑换码加载失败' },
  enabledToggled: {
    success: (isEnabled: boolean) => (isEnabled ? '兑换码已启用' : '兑换码已停用'),
  },
  toggleFailed: {
    error: (isEnabled: boolean) => (isEnabled ? '启用兑换码失败' : '停用兑换码失败'),
  },
  deleted: { success: '兑换码已删除' },
  deleteFailed: { error: '删除兑换码失败' },
  saveFailed: { error: (isEditing: boolean) => (isEditing ? '更新兑换码失败' : '创建兑换码失败') },
  redemptionsLoadFailed: { error: '兑换记录加载失败，请重试。' },
  redemptionDetailFailed: { error: '兑换详情加载失败。' },
}

export function useRewardCodesFeedback(): RewardCodesFeedback {
  return defineFeedback<RewardCodesFeedback>(REWARD_CODES_COPY)
}
