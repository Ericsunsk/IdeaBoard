import { computed, ref } from 'vue'
import { formatAdminDateTime } from '../../../shared/utils/dateTime'
import type { Task } from '../../../shared/api/task'
import type { AdminTaskResultFeedback } from './useAdminTaskMonitorFeedback'
import { getSafeResultImageUrl } from './taskResultImages'
import {
  formatTaskDuration,
  formatTaskResultDebug,
  formatTaskResultSummary,
  getTaskErrorReason,
  getTaskResultDetailImageUrl,
} from './taskResultPresentation'

export function useTaskResultDetail(options: { feedback: AdminTaskResultFeedback }) {
  const detailModalVisible = ref(false)
  const detailTask = ref<Task | null>(null)
  const showDebugResult = ref(false)

  const resultImageUrl = computed(() => {
    return getTaskResultDetailImageUrl(detailTask.value)
  })

  const resultSummaryText = computed(() => {
    return formatTaskResultSummary(detailTask.value?.result)
  })

  const resultDebugText = computed(() => {
    return formatTaskResultDebug(detailTask.value?.result)
  })

  function openResultImage(url: string) {
    if (!url.trim()) return
    const normalizedUrl = getSafeResultImageUrl(url)
    if (!normalizedUrl) {
      options.feedback.unsafeImageUrlRejected()
      return
    }
    window.open(normalizedUrl, '_blank', 'noopener,noreferrer')
  }

  function copyToClipboard(text: string) {
    if (!text) return
    navigator.clipboard
      .writeText(text)
      .then(() => options.feedback.copiedToClipboard())
      .catch(() => {
        options.feedback.copyFailed()
      })
  }

  function formatDateTime(timeStr: string | null | undefined): string {
    if (!timeStr) return '-'
    return formatAdminDateTime(timeStr, timeStr)
  }

  function showDetail(row: Task) {
    detailTask.value = row
    showDebugResult.value = false
    detailModalVisible.value = true
  }

  function getErrorReason(row: Task): string {
    return getTaskErrorReason(row)
  }

  function calcDuration(row: Task): string {
    return formatTaskDuration(row)
  }

  return {
    detailModalVisible,
    detailTask,
    showDebugResult,
    resultImageUrl,
    resultSummaryText,
    resultDebugText,
    openResultImage,
    copyToClipboard,
    formatDateTime,
    showDetail,
    getErrorReason,
    calcDuration,
  }
}
