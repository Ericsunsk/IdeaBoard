import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery, type InfiniteData, type QueryKey } from '@tanstack/vue-query'
import { PAGE_SIZES } from '../config/pagination'
import { noRetry } from './queryDefaults'

type CursorPage = {
  next_cursor?: string | null | undefined
}

/** Account feeds use standard pages, `next_cursor` pagination and no automatic retries. */
export function useCursorInfiniteList<TPage extends CursorPage, TItem>(options: {
  queryKey: MaybeRefOrGetter<QueryKey>
  fetchPage: (args: {
    pageParam: string
    signal: AbortSignal
    queryKey: QueryKey
    pageSize: number
  }) => Promise<TPage>
  isEnabled?: MaybeRefOrGetter<boolean>
  getItems: (page: TPage) => readonly TItem[]
}) {
  const query = useInfiniteQuery<TPage, Error, InfiniteData<TPage>, QueryKey, string>({
    queryKey: computed(() => toValue(options.queryKey)),
    queryFn: ({ pageParam, signal, queryKey }) =>
      options.fetchPage({ pageParam, signal, queryKey, pageSize: PAGE_SIZES.standard }),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    ...(options.isEnabled !== undefined ? { enabled: options.isEnabled } : {}),
    ...noRetry(),
  })

  const items = computed(
    () => query.data.value?.pages.flatMap((page) => options.getItems(page)) ?? [],
  )
  const hasLoadedPage = computed(() => query.data.value !== undefined)
  const loadMoreError = computed(() =>
    query.isFetchNextPageError.value ? query.error.value : null,
  )

  return {
    query,
    items,
    hasLoadedPage,
    loadMoreError,
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    retryInitial: query.refetch,
    loadMore: query.fetchNextPage,
  }
}
