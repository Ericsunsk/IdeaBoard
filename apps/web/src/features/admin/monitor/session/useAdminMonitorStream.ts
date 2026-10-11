import { computed, onScopeDispose, ref } from 'vue'
import type { MonitorResponse } from '../resource'

type AdminMonitorStreamStatus = 'idle' | 'unsupported' | 'connecting' | 'connected' | 'degraded'

// Global monitor snapshot stream (W2 transports, fail-closed). The legacy
// `GET /api/admin/monitor/stream` SSE endpoint is deleted and the Control
// Plane has no global stream equivalent (workspace events are
// per-workspace), so this hook never opens a socket. The state machine and
// hook shape stay intact: consumers keep polling through
// `useAdminTaskRuntimeSnapshot` (Control monitor read) with their existing
// loading/error states. Backend gap: global monitor stream (see the stream
// report).
export function useAdminMonitorStream() {
  const status = ref<AdminMonitorStreamStatus>('idle')
  const snapshot = ref<MonitorResponse | null>(null)
  const isConnected = computed(() => false)
  const isSupported = typeof EventSource !== 'undefined'

  function connectStream() {
    if (!isSupported) {
      status.value = 'unsupported'
      return
    }
    // No transport: stay idle and let the polling fallback serve snapshots.
    status.value = 'idle'
  }

  function disconnectStream(nextStatus: AdminMonitorStreamStatus = 'idle') {
    status.value = nextStatus
  }

  function reconnect() {
    disconnectStream()
    connectStream()
  }

  onScopeDispose(() => disconnectStream())

  return {
    isConnected,
    reconnect,
    snapshot,
    status,
  }
}
