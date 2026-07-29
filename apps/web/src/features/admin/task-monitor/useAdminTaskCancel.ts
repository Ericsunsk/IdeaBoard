import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { ref } from 'vue'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import type { AdminTaskRow } from './useAdminTaskList'
import type { AdminTaskCancellationFeedback } from './useAdminTaskMonitorFeedback'
import {
  cancelAdminTaskByID,
  getAdminMonitorQueryKey,
  getAdminTaskDetailQueryKey,
  getAdminTaskListQueryKey,
} from './taskMonitorResource'

type UseAdminTaskCancelOptions = {
  confirm: AdminConfirmPort
  feedback: AdminTaskCancellationFeedback
}

export function useAdminTaskCancel(options: UseAdminTaskCancelOptions) {
  const queryClient = useQueryClient()
  const pendingTaskID = ref<string | null>(null)
  const cancelMutation = useMutation({
    mutationFn: cancelAdminTaskByID,
    onSuccess: async (_result, taskID) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getAdminTaskListQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getAdminMonitorQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getAdminTaskDetailQueryKey(taskID) }),
      ])
    },
  })

  function isCancellableTask(task: AdminTaskRow) {
    return task.status === 'pending' || task.status === 'running'
  }

  function isCancellingTask(taskID: string) {
    return pendingTaskID.value === taskID
  }

  function requestCancelTask(task: AdminTaskRow) {
    if (!isCancellableTask(task) || pendingTaskID.value) return
    options.confirm.request({
      title: '确认取消任务',
      message: '任务将停止执行，已产生的结果和账务记录会保留。',
      confirmLabel: '取消任务',
      intent: 'warn',
      onConfirm: () => cancelTask(task.task_id),
    })
  }

  async function cancelTask(taskID: string) {
    if (pendingTaskID.value) return
    pendingTaskID.value = taskID
    try {
      await cancelMutation.mutateAsync(taskID)
      options.feedback.taskCancelled()
    } catch (error) {
      options.feedback.cancelTaskFailed(error)
    } finally {
      pendingTaskID.value = null
    }
  }

  return {
    isCancellableTask,
    isCancellingTask,
    requestCancelTask,
  }
}
