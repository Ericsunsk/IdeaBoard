import { nextTick, ref, toValue, watch, type Ref, type WatchSource } from 'vue'
import { useSortable } from '@vueuse/integrations/useSortable'

export interface SortableMediaInput {
  id: string
}

interface UseSortableMediaInputsOptions<T extends SortableMediaInput> {
  containerRef: Ref<HTMLElement | null>
  sourceItems: WatchSource<readonly T[]>
  draggableSelector: string
  filterSelector?: string
  fallbackClass: string
  onUpdate: (items: T[]) => void
}

type SortableReorderEvent = {
  oldDraggableIndex?: number | null
  newDraggableIndex?: number | null
  oldIndex?: number | null
  newIndex?: number | null
}

function areItemsInSameOrder<T extends SortableMediaInput>(
  left: readonly T[],
  right: readonly T[],
): boolean {
  return left.length === right.length && left.every((item, index) => item.id === right[index]?.id)
}

function resolveSortableIndex(
  primary: number | null | undefined,
  fallback: number | null | undefined,
): number | null {
  const index = primary ?? fallback
  if (typeof index !== 'number' || !Number.isInteger(index) || index < 0) return null
  return index
}

function moveSortableItem<T>(items: readonly T[], from: number, to: number) {
  if (from === to) return [...items]
  if (from < 0 || to < 0 || from >= items.length || to >= items.length) return null

  const next = [...items]
  const [item] = next.splice(from, 1)
  if (item === undefined) return null
  next.splice(to, 0, item)
  return next
}

export function useSortableMediaInputs<T extends SortableMediaInput>({
  containerRef,
  sourceItems,
  draggableSelector,
  filterSelector,
  fallbackClass,
  onUpdate,
}: UseSortableMediaInputsOptions<T>) {
  const sortableItems = ref<T[]>([]) as Ref<T[]>
  const isReordering = ref(false)

  function getSourceItems(): T[] {
    return [...toValue(sourceItems)]
  }

  function finishReorder() {
    void nextTick(() => {
      isReordering.value = false
      sortableItems.value = getSourceItems()
    })
  }

  function commitSortableUpdate(event: SortableReorderEvent) {
    const currentItems = getSourceItems()
    const from = resolveSortableIndex(event.oldDraggableIndex, event.oldIndex)
    const to = resolveSortableIndex(event.newDraggableIndex, event.newIndex)
    if (from === null || to === null) {
      finishReorder()
      return
    }

    const next = moveSortableItem(currentItems, from, to)
    if (!next || next.length !== currentItems.length || areItemsInSameOrder(next, currentItems)) {
      finishReorder()
      return
    }

    sortableItems.value = next
    onUpdate(next)
  }

  watch(
    sourceItems,
    (items) => {
      if (isReordering.value) return
      if (areItemsInSameOrder(sortableItems.value, items)) return
      sortableItems.value = [...items]
    },
    { immediate: true },
  )

  useSortable(() => containerRef.value, sortableItems, {
    animation: 150,
    draggable: draggableSelector,
    filter: filterSelector,
    ghostClass: 'is-sortable-ghost',
    chosenClass: 'is-sortable-chosen',
    dragClass: 'is-sortable-drag',
    fallbackClass,
    forceFallback: true,
    fallbackOnBody: true,
    fallbackTolerance: 4,
    preventOnFilter: false,
    onStart() {
      isReordering.value = true
    },
    onUpdate(event: SortableReorderEvent) {
      commitSortableUpdate(event)
    },
    onEnd() {
      finishReorder()
    },
  } as Parameters<typeof useSortable<T>>[2])

  return {
    isReordering,
    sortableItems,
  }
}
