import { ref } from 'vue'

type PollableRun = {
  id: number
  status: string
}

export function useAdminCreditRunDetail<TRun extends PollableRun>(
  load: (runId: number, signal: AbortSignal) => Promise<TRun>,
) {
  const selectedRunId = ref<number | null>(null)
  const run = ref<TRun>()
  const loading = ref(false)
  const error = ref(false)
  let requestEpoch = 0
  let activeRequest: AbortController | undefined
  let pollTimer: ReturnType<typeof setTimeout> | undefined

  async function open(runId: number) {
    selectedRunId.value = runId
    return refresh()
  }

  async function refresh() {
    const runId = selectedRunId.value
    if (runId === null) return

    const epoch = ++requestEpoch
    activeRequest?.abort()
    const controller = new AbortController()
    activeRequest = controller
    stopPolling()
    loading.value = true
    error.value = false
    run.value = undefined

    try {
      const nextRun = await load(runId, controller.signal)
      if (controller.signal.aborted || epoch !== requestEpoch || selectedRunId.value !== runId)
        return
      run.value = nextRun
      if (nextRun.status === 'queued' || nextRun.status === 'running') schedulePolling()
    } catch {
      if (!controller.signal.aborted && epoch === requestEpoch && selectedRunId.value === runId) {
        error.value = true
      }
    } finally {
      if (!controller.signal.aborted && epoch === requestEpoch && selectedRunId.value === runId) {
        loading.value = false
      }
    }
  }

  function close() {
    requestEpoch += 1
    activeRequest?.abort()
    activeRequest = undefined
    stopPolling()
    selectedRunId.value = null
    run.value = undefined
    loading.value = false
    error.value = false
  }

  function schedulePolling() {
    pollTimer = setTimeout(() => {
      void refresh()
    }, 3_000)
  }

  function stopPolling() {
    if (pollTimer !== undefined) clearTimeout(pollTimer)
    pollTimer = undefined
  }

  return { selectedRunId, run, loading, error, open, refresh, close }
}
