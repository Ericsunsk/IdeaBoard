import {
  computed,
  readonly,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import { normalizePageSize } from '../../../../core/config/pagination'

type AdminPagePaginationOptions = {
  currentPage: Ref<number>
  totalRecords: MaybeRefOrGetter<number | null | undefined>
  pageSize: MaybeRefOrGetter<number>
  pending?: MaybeRefOrGetter<boolean>
}

export function usePagePagination(options: AdminPagePaginationOptions) {
  const totalPages = shallowRef(1)
  const isPending = computed(() => Boolean(options.pending && toValue(options.pending)))
  const canPrevious = computed(() => options.currentPage.value > 1 && !isPending.value)
  const canNext = computed(() => options.currentPage.value < totalPages.value && !isPending.value)

  watch(
    [() => toValue(options.totalRecords), () => toValue(options.pageSize)],
    ([rawTotalRecords, rawPageSize]) => {
      if (!Number.isSafeInteger(options.currentPage.value) || options.currentPage.value < 1) {
        options.currentPage.value = 1
      }

      const totalRecords = normalizeTotalRecords(rawTotalRecords)
      if (totalRecords === undefined) return

      const nextTotalPages = Math.max(
        1,
        Math.ceil(totalRecords / normalizePageSize(rawPageSize, 1, Number.POSITIVE_INFINITY)),
      )
      totalPages.value = nextTotalPages
      if (options.currentPage.value > nextTotalPages) {
        options.currentPage.value = nextTotalPages
      }
    },
    { flush: 'sync', immediate: true },
  )

  function previous() {
    if (!canPrevious.value) return false
    options.currentPage.value -= 1
    return true
  }

  function next() {
    if (!canNext.value) return false
    options.currentPage.value += 1
    return true
  }

  function reset() {
    if (options.currentPage.value === 1) return false
    options.currentPage.value = 1
    return true
  }

  return {
    currentPage: readonly(options.currentPage),
    totalPages: readonly(totalPages),
    canPrevious,
    canNext,
    previous,
    next,
    reset,
  }
}

function normalizeTotalRecords(value: number | null | undefined) {
  if (value == null) return undefined
  return Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
}
