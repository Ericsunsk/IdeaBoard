import { reactive } from 'vue'
import { readConfirmReason, useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { useRewardCodesFeedback } from './useRewardCodesFeedback'
import {
  createAdminRewardCodeResource,
  type AdminRewardCodeResource,
  type RewardCode,
  type RewardCodePage,
} from './resource'

export function useRewardCodeRowActions(options: {
  resource?: AdminRewardCodeResource
  updateRewardCodes: (update: (page: RewardCodePage) => RewardCodePage) => void
  refreshRewardCodes: () => void
}) {
  const resource = options.resource ?? createAdminRewardCodeResource()
  const feedback = useRewardCodesFeedback()
  const confirm = useAdminConfirm()
  const enabledPending = reactive<Record<number, boolean>>({})
  const deletePending = reactive<Record<number, boolean>>({})

  function replaceRewardCode(updated: RewardCode) {
    options.updateRewardCodes((page) => ({
      ...page,
      rewardCodes: page.rewardCodes.map((candidate) =>
        candidate.id === updated.id ? updated : candidate,
      ),
    }))
  }

  async function setRewardCodeEnabled(rewardCode: RewardCode, enabled: boolean) {
    if (enabledPending[rewardCode.id]) return
    const confirmed = await confirm.confirm({
      title: enabled ? '启用兑换码' : '停用兑换码',
      message: `确定${enabled ? '启用' : '停用'}“${rewardCode.displayName}”吗？该操作将记录审计。`,
      confirmLabel: enabled ? '确认启用' : '确认停用',
      intent: 'warn',
      reason: { required: true },
    })
    if (!confirmed) return
    const operatorReason = (confirm.readReason?.() ?? readConfirmReason()).trim()
    enabledPending[rewardCode.id] = true
    try {
      replaceRewardCode(await resource.setEnabled(rewardCode.id, enabled, { operatorReason }))
      feedback.enabledToggled(enabled)
      options.refreshRewardCodes()
    } catch (error) {
      feedback.toggleFailed(error, enabled)
    } finally {
      enabledPending[rewardCode.id] = false
    }
  }

  async function deleteRewardCode(rewardCode: RewardCode, operatorReason: string) {
    if (deletePending[rewardCode.id]) return
    deletePending[rewardCode.id] = true
    try {
      await resource.delete(rewardCode.id, { operatorReason })
      options.updateRewardCodes((page) => ({
        ...page,
        rewardCodes: page.rewardCodes.filter((candidate) => candidate.id !== rewardCode.id),
      }))
      feedback.deleted()
      options.refreshRewardCodes()
    } catch (error) {
      feedback.deleteFailed(error)
    } finally {
      deletePending[rewardCode.id] = false
    }
  }

  async function confirmDelete(rewardCode: RewardCode) {
    if (deletePending[rewardCode.id]) return
    const confirmed = await confirm.confirm({
      title: '删除兑换码',
      message: `确定永久删除“${rewardCode.displayName}”吗？没有 Redemption 的兑换码才可删除。`,
      confirmLabel: '确认删除',
      intent: 'danger',
      reason: { required: true },
    })
    if (!confirmed) return
    const operatorReason = (confirm.readReason?.() ?? readConfirmReason()).trim()
    await deleteRewardCode(rewardCode, operatorReason)
  }

  return {
    enabledPending,
    deletePending,
    replaceRewardCode,
    setRewardCodeEnabled,
    confirmDelete,
  }
}
