import { reactive } from 'vue'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import { useOptimisticToggle } from '../../../shared'
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
  const confirm = useConfirm()
  // 行开关乐观收口到共享 useOptimisticToggle：intent 同 tick 翻转，
  // pending 只压开关；成功后保留覆盖等回填 reconcile，失败回滚。
  // Status 胶囊四态仍跟服务端 status，投影只覆盖 enabled。
  const toggle = useOptimisticToggle<number>()
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
    await toggle.toggle(
      rewardCode.id,
      enabled,
      async (key, next) => {
        const updated = await resource.setEnabled(key, next)
        replaceRewardCode(updated)
        toggle.reconcile(key, updated.enabled)
      },
      {
        onSuccess: (_, next) => {
          feedback.enabledToggled(next)
          options.refreshRewardCodes()
        },
        onFailure: (_, next, error) => {
          feedback.toggleFailed(error, next)
        },
      },
    )
  }

  async function deleteRewardCode(rewardCode: RewardCode) {
    if (deletePending[rewardCode.id]) return
    deletePending[rewardCode.id] = true
    try {
      await resource.delete(rewardCode.id)
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
    })
    if (!confirmed) return

    await deleteRewardCode(rewardCode)
  }

  return {
    toggle,
    deletePending,
    replaceRewardCode,
    setRewardCodeEnabled,
    confirmDelete,
  }
}
