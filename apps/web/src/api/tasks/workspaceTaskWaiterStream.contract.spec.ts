import { it, expect, vi, afterEach } from 'vitest'
import { waitWorkspaceTaskTerminal } from './workspaceTaskWaiter'
import { resetWorkspaceEventStreamsForTests } from '../realtime/workspaceEventStream'
const state = vi.hoisted(() => ({ reads: 0, status: 'running' }))
vi.mock('./workspaceTaskTransport', () => ({
  getWorkspaceTask: async () => {
    state.reads++
    const snapshot = { id: 'task_abc123', workspace_id: 'ws_abc123', status: state.status }
    // GET snapshots running; worker commits completion before client attaches its stream listener.
    state.status = 'completed'
    return snapshot
  },
  isWorkspaceTaskGoneError: () => false,
}))
class HealthyEventSource {
  onerror = null
  addEventListener() {}
  close() {}
}
afterEach(() => {
  resetWorkspaceEventStreamsForTests()
  vi.unstubAllGlobals()
})
it('reconciles a task completed between initial GET snapshot and stream subscription', async () => {
  vi.stubGlobal('EventSource', HealthyEventSource)
  const pending = waitWorkspaceTaskTerminal('ws_abc123', 'task_abc123', { timeoutMs: 150 })
  await expect(pending).resolves.toMatchObject({ status: 'completed' })
  expect(state.reads).toBe(2)
})
