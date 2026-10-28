import { defineFeedback } from '../../shared'

export type AdminTaskResultFeedback = {
  unsafeImageUrlRejected: () => void
  copiedToClipboard: () => void
  copyFailed: () => void
}
export type AdminTaskCancellationFeedback = {
  taskCancelled: () => void
  cancelTaskFailed: (error: unknown) => void
}
export type AdminTaskLoadFeedback = {
  snapshotLoadFailed: (error: unknown) => void
  listLoadFailed: (error: unknown) => void
  detailLoadFailed: (error: unknown) => void
}

const ADMIN_TASK_RESULT_COPY = {
  unsafeImageUrlRejected: { errorText: '图片链接不安全，已拒绝打开' },
  copiedToClipboard: { success: '已复制到剪贴板' },
  copyFailed: { errorText: '复制失败' },
}

const ADMIN_TASK_CANCELLATION_COPY = {
  taskCancelled: { success: '任务已取消' },
  cancelTaskFailed: { error: '取消任务失败' },
}

const ADMIN_TASK_LOAD_COPY = {
  snapshotLoadFailed: { error: '加载任务运行监控失败' },
  listLoadFailed: { error: '加载任务列表失败' },
  detailLoadFailed: { error: '加载任务详情失败' },
}

export function useAdminTaskResultFeedback(): AdminTaskResultFeedback {
  return defineFeedback<AdminTaskResultFeedback>(ADMIN_TASK_RESULT_COPY)
}

export function useAdminTaskCancellationFeedback(): AdminTaskCancellationFeedback {
  return defineFeedback<AdminTaskCancellationFeedback>(ADMIN_TASK_CANCELLATION_COPY)
}

export function useAdminTaskLoadFeedback(): AdminTaskLoadFeedback {
  return defineFeedback<AdminTaskLoadFeedback>(ADMIN_TASK_LOAD_COPY)
}
