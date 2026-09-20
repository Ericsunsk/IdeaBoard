import { formatAdminDisplayError, type AdminToastPort, useAdminToast } from '../../shared/lib/toast'

export type AdminTaskResultFeedback = {
  unsafeImageUrlRejected: () => void
  copiedToClipboard: () => void
  copyFailed: () => void
}
export type AdminTaskCancellationFeedback = {
  taskCancelled: () => void
  cancelTaskFailed: (error: unknown) => void
}

function createAdminTaskResultFeedback(toast: AdminToastPort): AdminTaskResultFeedback {
  return {
    unsafeImageUrlRejected: () => toast.error('图片链接不安全，已拒绝打开'),
    copiedToClipboard: () => toast.success('已复制到剪贴板'),
    copyFailed: () => toast.error('复制失败'),
  }
}

export function useAdminTaskResultFeedback() {
  return createAdminTaskResultFeedback(useAdminToast())
}

export function useAdminTaskCancellationFeedback(): AdminTaskCancellationFeedback {
  const toast = useAdminToast()
  return {
    taskCancelled: () => toast.success('任务已取消'),
    cancelTaskFailed: (error) => toast.error(formatAdminDisplayError(error, '取消任务失败')),
  }
}
