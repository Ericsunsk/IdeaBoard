import { computed, type MaybeRefOrGetter } from 'vue'
import { useAdminCollectionQuery } from './useAdminCollectionQuery'
import { useCursorPagination, type AdminCursorValue } from './useCursorPagination'

/**
 * Converged cursor-based Admin collection lifecycle.
 *
 * Fuses `useCursorPagination` + key building + `useAdminCollectionQuery` into
 * the shape previously copy-pasted across the reward-code / redemption /
 * order lists: pagination is created before the query because the key reads
 * the cursor during setup, and filter state feeds the key so navigation
 * actions below only mutate state. Offset-based lists (users) keep their own
 * wiring and do not use this helper.
 */
export function useAdminCursorCollection<TPage, TCursor extends AdminCursorValue>(options: {
  buildKey: (cursor: TCursor | undefined) => readonly unknown[]
  queryFn: (context: { queryKey: readonly unknown[]; signal: AbortSignal }) => Promise<TPage>
  getNextCursor: (page: TPage) => TCursor | undefined
  enabled?: MaybeRefOrGetter<boolean>
}) {
  // Pagination is created before the query because the key reads the cursor
  // during setup; the explicit annotation breaks the inference cycle with the
  // `pending` closure below (same pattern the call sites used inline before).
  const pagination: ReturnType<typeof useCursorPagination<TCursor>> = useCursorPagination<TCursor>({
    pending: () => loading.value,
  })
  const queryKey = computed(() => options.buildKey(pagination.cursor.value))
  const { data, loading, initialError, backgroundError, refresh, query } = useAdminCollectionQuery<
    TPage,
    TCursor
  >({
    queryKey,
    queryFn: options.queryFn,
    ...(options.enabled !== undefined ? { enabled: options.enabled } : {}),
    getNextCursor: options.getNextCursor,
    pagination,
  })

  function previousPage() {
    pagination.retreat()
  }

  function nextPage() {
    pagination.advance()
  }

  return {
    data,
    loading,
    initialError,
    backgroundError,
    refresh,
    query,
    queryKey,
    pagination,
    previousPage,
    nextPage,
  }
}
