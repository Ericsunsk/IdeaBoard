import { formatAdminDisplayError, type AdminToastPort, useAdminToast } from '../../shared/lib/toast'

// 反馈公约（admin，见 settings useSettingsFeedback 文件头）：领域文案收口到本文件
// 的 feedback 工厂，调用方只表达意图，不直接拼 toast 文案。

export type RewardCodesFeedback = {
  loadFailed: (error: unknown) => void
  enabledToggled: (enabled: boolean) => void
  toggleFailed: (error: unknown, enabled: boolean) => void
  deleted: () => void
  deleteFailed: (error: unknown) => void
}

function createRewardCodesFeedback(toast: AdminToastPort): RewardCodesFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '兑换码加载失败')),
    enabledToggled: (enabled) => toast.success(enabled ? '兑换码已启用' : '兑换码已停用'),
    toggleFailed: (error, enabled) =>
      toast.error(formatAdminDisplayError(error, enabled ? '启用兑换码失败' : '停用兑换码失败')),
    deleted: () => toast.success('兑换码已删除'),
    deleteFailed: (error) => toast.error(formatAdminDisplayError(error, '删除兑换码失败')),
  }
}

export function useRewardCodesFeedback() {
  return createRewardCodesFeedback(useAdminToast())
}
