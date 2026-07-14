import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, type Ref } from 'vue'
import type { AdminTaskDeletionFeedback } from './useAdminTaskMonitorFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { adminTaskStatuses } from './taskMonitorStatus'
import {
  deleteAdminTaskSelection,
  getAdminMonitorQueryKey,
  getAdminTaskListQueryKey,
} from './taskMonitorResource'
import type { AdminTaskRow } from './useAdminTaskList'

type UseAdminTaskSelectionDeleteOptions = {
  selectedTasks: Ref<AdminTaskRow[]>
  confirm: AdminConfirmPort
  feedback: AdminTaskDeletionFeedback
}

export function useAdminTaskSelectionDelete(options: UseAdminTaskSelectionDeleteOptions) {
  const queryClient = useQueryClient()
  const isDeletingSelectedTasks = ref(false)
  const deleteMutation = useMutation({
    mutationFn: deleteAdminTaskSelection,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getAdminTaskListQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getAdminMonitorQueryKey() }),
      ])
    },
  })

  const selectedTaskIds = computed(() => normalizeSelectedTaskIDs(options.selectedTasks.value))
  const selectedActiveTaskCount = computed(
    () => options.selectedTasks.value.filter((task) => isActiveTask(task)).length,
  )
  const canDeleteSelectedTasks = computed(
    () => selectedTaskIds.value.length > 0 && !isDeletingSelectedTasks.value,
  )

  function requestDeleteSelectedTasks() {
    const taskIDs = selectedTaskIds.value
    if (taskIDs.length <= 0) {
      options.feedback.noTasksSelected()
      return
    }

    const activeCount = selectedActiveTaskCount.value
    options.confirm.request({
      title: '确认删除选中任务',
      message:
        activeCount > 0
          ? `将删除 ${taskIDs.length} 个任务，其中 ${activeCount} 个仍在等待或执行中。系统会先停止这些任务，再删除记录。此操作不可撤销。`
          : `将删除 ${taskIDs.length} 个任务。此操作不可撤销。`,
      confirmLabel: '删除任务',
      intent: 'danger',
      onConfirm: () => deleteSelectedTasks(taskIDs),
    })
  }

  async function deleteSelectedTasks(taskIDs: string[]) {
    isDeletingSelectedTasks.value = true
    try {
      const result = await deleteMutation.mutateAsync(taskIDs)
      options.selectedTasks.value = []
      options.feedback.tasksDeleted(result.deleted_count)
    } catch (error) {
      options.feedback.deleteTasksFailed(error)
    } finally {
      isDeletingSelectedTasks.value = false
    }
  }

  return {
    canDeleteSelectedTasks,
    isDeletingSelectedTasks,
    requestDeleteSelectedTasks,
    selectedActiveTaskCount,
    selectedTaskIds,
  }
}

function normalizeSelectedTaskIDs(tasks: AdminTaskRow[]) {
  const seen = new Set<string>()
  const taskIDs: string[] = []
  for (const task of tasks) {
    const taskID = task.task_id.trim()
    if (!taskID || seen.has(taskID)) continue
    seen.add(taskID)
    taskIDs.push(taskID)
  }
  return taskIDs
}

function isActiveTask(task: AdminTaskRow) {
  return task.status === adminTaskStatuses.pending || task.status === adminTaskStatuses.running
}
