import { useDocumentVisibility } from '@vueuse/core'
import { computed, onScopeDispose, ref, watch } from 'vue'
import {
  getGetAdminMonitorQueryKey,
} from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import type { ApiResponseMonitor } from '../../../../shared/api/generated-query/model'
import { resolveApiUrl } from '../../../../shared/config/runtimeUrls'

type AdminMonitorStreamStatus =
  | 'idle'
  | 'unsupported'
  | 'connecting'
  | 'connected'
  | 'degraded'

function parseMonitorSnapshot(raw: string): ApiResponseMonitor | null {
  const payload = JSON.parse(raw) as unknown
  if (!payload || typeof payload !== 'object') return null
  const response = payload as Partial<ApiResponseMonitor>
  if (typeof response.code !== 'number' || typeof response.msg !== 'string') return null
  if (response.code !== 0 || !response.data?.monitor || !response.data?.stats) return null
  return response as ApiResponseMonitor
}

export function useTaskMonitorStream() {
  const queryClient = useQueryClient()
  const visibility = useDocumentVisibility()
  const status = ref<AdminMonitorStreamStatus>('idle')
  const snapshot = ref<ApiResponseMonitor | null>(null)
  const lastSnapshotAt = ref<number | null>(null)
  const isPageVisible = computed(() => visibility.value === 'visible')
  const isConnected = computed(() => status.value === 'connected')
  let eventSource: EventSource | null = null

  function closeStream(nextStatus: AdminMonitorStreamStatus = 'idle') {
    eventSource?.close()
    eventSource = null
    status.value = nextStatus
  }

  function connectStream() {
    if (typeof EventSource === 'undefined') {
      status.value = 'unsupported'
      return
    }
    if (eventSource) return

    status.value = 'connecting'
    const source = new EventSource(resolveApiUrl('/admin/monitor/stream'), {
      withCredentials: true,
    })
    eventSource = source

    source.onopen = () => {
      status.value = 'connected'
    }

    source.addEventListener('snapshot', (event: MessageEvent) => {
      try {
        const nextSnapshot = parseMonitorSnapshot(event.data)
        if (!nextSnapshot) {
          status.value = 'degraded'
          return
        }
        queryClient.setQueryData(getGetAdminMonitorQueryKey(), nextSnapshot)
        snapshot.value = nextSnapshot
        lastSnapshotAt.value = Date.now()
        status.value = 'connected'
      } catch {
        status.value = 'degraded'
      }
    })

    source.addEventListener('snapshot_error', () => {
      status.value = 'degraded'
    })

    source.addEventListener('close', () => {
      closeStream('idle')
      if (isPageVisible.value) connectStream()
    })

    source.onerror = () => {
      status.value = 'degraded'
    }
  }

  watch(
    isPageVisible,
    (visible) => {
      if (visible) {
        connectStream()
        return
      }
      closeStream('idle')
    },
    { immediate: true },
  )

  onScopeDispose(() => closeStream())

  return {
    isConnected,
    lastSnapshotAt,
    snapshot,
    status,
  }
}
