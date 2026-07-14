import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../shared/lib/adminToast'

export type AdminTaskResultFeedback = {
  unsafeImageUrlRejected: () => void
  copiedToClipboard: () => void
  copyFailed: () => void
}

export type AdminTaskDeletionFeedback = {
  noTasksSelected: () => void
  tasksDeleted: (deletedCount: number) => void
  deleteTasksFailed: (error: unknown) => void
}

export function createAdminTaskResultFeedback(toast: AdminToastPort): AdminTaskResultFeedback {
  return {
    unsafeImageUrlRejected: () => toast.error('图片链接不安全，已拒绝打开'),
    copiedToClipboard: () => toast.success('已复制到剪贴板'),
    copyFailed: () => toast.error('复制失败'),
  }
}

export function createAdminTaskDeletionFeedback(toast: AdminToastPort): AdminTaskDeletionFeedback {
  return {
    noTasksSelected: () => toast.warning('请选择要删除的任务'),
    tasksDeleted: (deletedCount) => {
      if (deletedCount <= 0) {
        toast.warning('没有可删除的任务，列表已刷新')
        return
      }
      toast.success(`已删除 ${deletedCount} 个任务`)
    },
    deleteTasksFailed: (error) => toast.error(formatAdminDisplayError(error, '删除任务失败')),
  }
}

export function useAdminTaskResultFeedback() {
  return createAdminTaskResultFeedback(useAdminToast())
}

export function useAdminTaskDeletionFeedback() {
  return createAdminTaskDeletionFeedback(useAdminToast())
}
