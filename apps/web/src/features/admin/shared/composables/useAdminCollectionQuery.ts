import { computed, watch, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { useQuery, type Query } from '@tanstack/vue-query'
import { noRetry } from '../../../../shared/query/queryDefaults'

/**
 * Shared TanStack Query owner for Admin list lifecycles.
 *
 * Converges the `queryKey / queryFn / retry:false / loading /
 * initialError-backgroundError / watch(page => setNextCursor)` shape that was
 * copy-pasted across the finding / run-history / reward-code / redemption /
 * order lists after `useCollectionState` retired. Callers keep ownership of
 * their `QUERY_ROOT` values, criteria shapes and pagination; this helper owns
 * only the query wiring so the collection-state contract stays uniform.
 */
export function useAdminCollectionQuery<TPage, TCursor>(options: {
  queryKey: MaybeRef<readonly unknown[]>
  queryFn: (context: { queryKey: readonly unknown[]; signal: AbortSignal }) => Promise<TPage>
  enabled?: MaybeRefOrGetter<boolean>
  // Value/getter forms (including the `useQueryPolling` getter) plus the
  // query-callback form from `shared/query/queryDefaults` presets. The bare
  // getter rides the callback branch structurally (fewer params), matching
  // how `useQuery` itself accepts it.
  refetchInterval?:
    MaybeRefOrGetter<number | false> | ((query: Query<TPage>) => number | false | undefined)
  getNextCursor?: (page: TPage) => TCursor | undefined
  pagination?: { setNextCursor: (cursor: TCursor | undefined) => boolean }
}) {
  const query = useQuery({
    queryKey: options.queryKey,
    queryFn: options.queryFn,
    ...noRetry(),
    ...(options.enabled !== undefined ? { enabled: options.enabled } : {}),
    ...(options.refetchInterval !== undefined ? { refetchInterval: options.refetchInterval } : {}),
  })
  const data = query.data
  const loading = computed(() => query.isFetching.value)
  const initialError = computed(() => query.isError.value && data.value === undefined)
  const backgroundError = computed(() => query.isError.value && data.value !== undefined)

  if (options.pagination && options.getNextCursor) {
    const { pagination, getNextCursor } = options
    watch(data, (page) => {
      if (page) pagination.setNextCursor(getNextCursor(page))
    })
  }

  function refresh() {
    return query.refetch()
  }

  return { data, loading, initialError, backgroundError, refresh, query }
}
