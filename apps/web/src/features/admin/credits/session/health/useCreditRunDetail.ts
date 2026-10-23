import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { adminPolling, noRetry } from '@/core/query/queryDefaults'

type PollableRun = {
  id: number
  status: string
}

const CREDIT_RUN_DETAIL_QUERY_ROOT = ['admin', 'credits', 'run-detail'] as const
const RUN_DETAIL_ID_INDEX = CREDIT_RUN_DETAIL_QUERY_ROOT.length
const RUN_DETAIL_POLL_MS = 3_000

function isPollableStatus(status: string): boolean {
  return status === 'queued' || status === 'running'
}

export function useCreditRunDetail<TRun extends PollableRun>(
  load: (runId: number, signal: AbortSignal) => Promise<TRun>,
) {
  const selectedRunId = ref<number | null>(null)
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: computed(() => [...CREDIT_RUN_DETAIL_QUERY_ROOT, selectedRunId.value]),
    queryFn: ({ queryKey, signal }) => load(queryKey[RUN_DETAIL_ID_INDEX] as number, signal),
    enabled: computed(() => selectedRunId.value !== null),
    // Single attempt like the previous hand-rolled loader: failures surface
    // as the detail error state instead of retrying behind the dialog.
    ...noRetry(),
    // Terminal runs settle; queued/running runs keep the 3s cadence.
    ...adminPolling(RUN_DETAIL_POLL_MS, false, (data) => {
      const run = data as TRun | undefined
      return run != null && isPollableStatus(run.status)
    }),
  })

  async function open(runId: number) {
    if (selectedRunId.value === runId) {
      // Same row re-expanded: the key is unchanged so force a fresh load,
      // matching the previous always-reload behavior.
      await query.refetch()
      return
    }
    selectedRunId.value = runId
  }

  function refresh() {
    return query.refetch()
  }

  function close() {
    const runId = selectedRunId.value
    selectedRunId.value = null
    if (runId !== null) {
      void queryClient.removeQueries({ queryKey: [...CREDIT_RUN_DETAIL_QUERY_ROOT, runId] })
    }
  }

  return {
    selectedRunId,
    run: computed(() => query.data.value),
    loading: computed(() => query.isLoading.value),
    error: computed(() => query.isError.value),
    open,
    refresh,
    close,
  }
}
