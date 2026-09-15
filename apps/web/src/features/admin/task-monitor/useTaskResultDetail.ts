import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useAppClipboard } from '../../../shared/composables/useAppClipboard'
import { formatAdminDateTime } from '../../../shared/utils/dateTime'
import type { AdminTaskResultFeedback } from './useAdminTaskMonitorFeedback'
import type { AdminTaskMonitorDetailModel } from './resource'
import {
  getSafeResultImageUrl,
  hasTaskResultImageCandidate,
  isTaskResultImageDeletedByUser,
} from './taskResultImages'
import {
  formatTaskDuration,
  formatTaskResultDebug,
  formatTaskResultSummary,
  getTaskErrorReason,
  getTaskResultDetailImageUrl,
} from './taskResultPresentation'

export function useTaskResultDetail(
  task: MaybeRefOrGetter<AdminTaskMonitorDetailModel | null>,
  options: { feedback: AdminTaskResultFeedback },
) {
  const detailTask = computed(() => toValue(task))
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

  const hasResultImageCandidate = computed(() => {
    return hasTaskResultImageCandidate(detailTask.value?.result)
  })

  const isResultImageDeletedByUser = computed(() =>
    isTaskResultImageDeletedByUser(detailTask.value?.result),
  )

  const hasResultPayload = computed(() => Boolean(detailTask.value?.result?.trim()))

  // Debug expansion is local UI state and must never follow a different task into the Dialog.
  watch(detailTask, () => {
    showDebugResult.value = false
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

  const { copyText } = useAppClipboard()

  function copyToClipboard(text: string) {
    if (!text) return
    copyText(text).then((ok) => {
      if (ok) options.feedback.copiedToClipboard()
      else options.feedback.copyFailed()
    })
  }

  function formatDateTime(timeStr: string | null | undefined): string {
    if (!timeStr) return '-'
    return formatAdminDateTime(timeStr, timeStr)
  }

  function getErrorReason(row: AdminTaskMonitorDetailModel): string {
    return getTaskErrorReason(row)
  }

  function calcDuration(row: AdminTaskMonitorDetailModel): string {
    return formatTaskDuration(row)
  }

  return {
    detailTask,
    showDebugResult,
    resultImageUrl,
    resultSummaryText,
    resultDebugText,
    hasResultImageCandidate,
    isResultImageDeletedByUser,
    hasResultPayload,
    openResultImage,
    copyToClipboard,
    formatDateTime,
    getErrorReason,
    calcDuration,
  }
}
