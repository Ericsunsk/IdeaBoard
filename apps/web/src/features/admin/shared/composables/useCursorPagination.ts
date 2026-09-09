import { computed, shallowRef, toValue, type MaybeRefOrGetter } from 'vue'

export type AdminCursorValue = string | number

export function useCursorPagination<TCursor extends AdminCursorValue>(
  options: { pending?: MaybeRefOrGetter<boolean> } = {},
) {
  const cursorStack = shallowRef<Array<TCursor | undefined>>([undefined])
  const nextCursor = shallowRef<TCursor>()

  const cursor = computed(() => cursorStack.value.at(-1))
  const currentPage = computed(() => cursorStack.value.length)
  const hasNext = computed(() => nextCursor.value !== undefined)
  const knownPageCount = computed(() => currentPage.value + (hasNext.value ? 1 : 0))
  const isPending = computed(() => Boolean(options.pending && toValue(options.pending)))
  const canAdvance = computed(() => hasNext.value && !isPending.value)
  const canRetreat = computed(() => currentPage.value > 1 && !isPending.value)

  function setNextCursor(value: TCursor | undefined) {
    if (
      value === undefined ||
      (typeof value === 'number' && (!Number.isSafeInteger(value) || value < 0)) ||
      cursorStack.value.includes(value)
    ) {
      nextCursor.value = undefined
      return false
    }
    nextCursor.value = value
    return true
  }

  function advance() {
    if (!canAdvance.value || nextCursor.value === undefined) return false
    cursorStack.value = [...cursorStack.value, nextCursor.value]
    nextCursor.value = undefined
    return true
  }

  function retreat() {
    if (!canRetreat.value) return false
    const previousStack = [...cursorStack.value]
    nextCursor.value = previousStack.pop()
    cursorStack.value = previousStack
    return true
  }

  function reset() {
    cursorStack.value = [undefined]
    nextCursor.value = undefined
  }

  return {
    cursor,
    currentPage,
    hasNext,
    knownPageCount,
    canAdvance,
    canRetreat,
    setNextCursor,
    advance,
    retreat,
    reset,
  }
}
