import { computed, ref } from 'vue'
import { taskApi } from '../api'
import type { Task } from '../api'
import { buildGalleryGroups } from '../utils/galleryGroups'
import { getTaskImageUrl, stripFeaturePrefix } from '../utils/task-history'

type PromptMatcher = (prompt?: string) => boolean

interface UseFeatureHistoryOptions {
  matchPrompt: PromptMatcher
  canLoad?: () => boolean
}

function isCompletedTaskWithImage(task: Task): boolean {
  return task.status === 'completed' && !!getTaskImageUrl(task)
}

export function useFeatureHistory(options: UseFeatureHistoryOptions) {
  const historyTasks = ref<Task[]>([])
  const activeTaskId = ref<string | null>(null)

  const galleryTasks = computed(() =>
    historyTasks.value.filter((task) => isCompletedTaskWithImage(task) && options.matchPrompt(task.prompt))
  )

  const allGalleryGroups = computed(() => buildGalleryGroups(historyTasks.value))

  async function loadHistory() {
    if (options.canLoad && !options.canLoad()) return
    try {
      const res: any = await taskApi.list({ page: 1, page_size: 200 })
      if (res.code === 0) {
        historyTasks.value = res.data?.list || []
      }
    } catch {}
  }

  return {
    historyTasks,
    activeTaskId,
    galleryTasks,
    allGalleryGroups,
    stripPrefix: stripFeaturePrefix,
    getTaskImageUrl,
    loadHistory,
  }
}
