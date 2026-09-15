import { useQueryClient } from '@tanstack/vue-query'
import { useDocumentVisibility, useEventSource } from '@vueuse/core'
import { computed, onScopeDispose, ref, watch } from 'vue'
import { getGetAdminMonitorQueryKey } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import type { ApiResponseMonitor, MonitorResponse } from '../../../shared/api/generated-query/model'
import { requireApiData } from '../../../shared/api/apiResponse'
import { resolveApiUrl } from '../../../shared/config/runtimeUrls'

type AdminMonitorStreamStatus = 'idle' | 'unsupported' | 'connecting' | 'connected' | 'degraded'

type ParsedMonitorSnapshot = {
  response: ApiResponseMonitor
  snapshot: MonitorResponse
}

function parseMonitorSnapshot(raw: string): ParsedMonitorSnapshot | null {
  const payload = JSON.parse(raw) as unknown
  if (!payload || typeof payload !== 'object') return null
  const response = payload as Partial<ApiResponseMonitor>
  if (typeof response.code !== 'number' || typeof response.msg !== 'string') return null
  const typedResponse = response as ApiResponseMonitor
  const snapshot = requireApiData(typedResponse, '读取监控快照失败')
  if (!snapshot.monitor || !snapshot.stats) return null
  return { response: typedResponse, snapshot }
}

export function useAdminTaskMonitorStream() {
  const queryClient = useQueryClient()
  const visibility = useDocumentVisibility()
  const status = ref<AdminMonitorStreamStatus>('idle')
  const snapshot = ref<MonitorResponse | null>(null)
  const isPageVisible = computed(() => visibility.value === 'visible')
  const isConnected = computed(() => status.value === 'connected')
  const isSupported = typeof EventSource !== 'undefined'

  // Transport owned by `@vueuse/core` (auto-reconnect); this module keeps the
  // `idle/unsupported/connecting/connected/degraded` contract and the Query
  // cache write. The non-Vue `/user/events` singleton in
  // `shared/api/userEventStream.ts` intentionally stays hand-rolled: promise
  // machinery (`taskEvents.ts`) has no component scope for a composable.
  const {
    data,
    event,
    status: sourceStatus,
    error,
    open,
    close,
  } = useEventSource(
    resolveApiUrl('/admin/monitor/stream'),
    ['snapshot', 'snapshot_error', 'close'],
    { immediate: false, autoReconnect: { retries: -1, delay: 1000 }, withCredentials: true },
  )

  function connectStream() {
    if (!isSupported) {
      status.value = 'unsupported'
      return
    }
    status.value = 'connecting'
    open()
  }

  function disconnectStream(nextStatus: AdminMonitorStreamStatus = 'idle') {
    close()
    status.value = nextStatus
  }

  function applySnapshot(raw: string | null | undefined) {
    if (raw == null) return
    try {
      const parsed = parseMonitorSnapshot(raw)
      if (!parsed) {
        status.value = 'degraded'
        return
      }
      queryClient.setQueryData(getGetAdminMonitorQueryKey(), parsed.response)
      snapshot.value = parsed.snapshot
      status.value = 'connected'
    } catch {
      status.value = 'degraded'
    }
  }

  watch([event, data], ([nextEvent]) => {
    if (nextEvent === 'snapshot') applySnapshot(data.value)
    else if (nextEvent === 'snapshot_error') status.value = 'degraded'
    else if (nextEvent === 'close') {
      // Server-initiated close: tear down and reconnect when visible.
      disconnectStream()
      if (isPageVisible.value) connectStream()
    }
  })

  // Transport state mirrors the contract; CLOSED is intentionally unmapped —
  // explicit closes set `idle` at the call site and auto-reconnect cycles
  // through CONNECTING/OPEN on its own.
  watch(sourceStatus, (next) => {
    if (next === 'CONNECTING') status.value = 'connecting'
    else if (next === 'OPEN') status.value = 'connected'
  })

  watch(error, (next) => {
    if (next) status.value = 'degraded'
  })

  function reconnect() {
    disconnectStream()
    if (isPageVisible.value) connectStream()
  }

  watch(
    isPageVisible,
    (visible) => {
      if (visible) {
        connectStream()
        return
      }
      disconnectStream()
    },
    { immediate: true },
  )

  onScopeDispose(() => close())

  return {
    isConnected,
    reconnect,
    snapshot,
    status,
  }
}
