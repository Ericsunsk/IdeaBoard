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

function areItemsInSameOrder<T extends SortableMediaInput>(
  left: readonly T[],
  right: readonly T[],
): boolean {
  return left.length === right.length && left.every((item, index) => item.id === right[index]?.id)
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
    onEnd() {
      const currentItems = getSourceItems()
      const imageIds = Array.from(
        containerRef.value?.querySelectorAll<HTMLElement>(draggableSelector) ?? [],
      )
        .map((item) => item.dataset.imageId ?? '')
        .filter(Boolean)
      if (imageIds.length !== currentItems.length) {
        finishReorder()
        return
      }

      const byId = new Map(currentItems.map((item) => [item.id, item]))
      const next = imageIds
        .map((imageId) => byId.get(imageId))
        .filter((item): item is T => !!item)
      if (next.length !== currentItems.length) {
        finishReorder()
        return
      }
      if (areItemsInSameOrder(next, currentItems)) {
        finishReorder()
        return
      }

      sortableItems.value = next
      onUpdate(next)
      finishReorder()
    },
  } as Parameters<typeof useSortable<T>>[2])

  return {
    isReordering,
    sortableItems,
  }
}
