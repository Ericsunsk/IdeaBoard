import { nextTick, ref, toValue, watch, type Ref, type WatchSource } from 'vue'
import { usePointerDragSession } from '../../../../session/browser/interaction/usePointerDragSession.ts'

export interface SortableMediaInput {
  id: string
}

interface UseSortableMediaInputsOptions<T extends SortableMediaInput> {
  containerRef: Ref<HTMLElement | null>
  sourceItems: WatchSource<readonly T[]>
  draggableSelector: string
  filterSelector?: string | undefined
  onUpdate: (items: T[]) => void
}

type SortableReorderEvent = {
  itemId: string
  element: HTMLElement
  startX: number
  startY: number
  isActive: boolean
  hoverElement: HTMLElement | null
  hoverItemId: string | null
}

const SORTABLE_DRAG_TOLERANCE_PX = 4

function areItemsInSameOrder<T extends SortableMediaInput>(
  left: readonly T[],
  right: readonly T[],
): boolean {
  return left.length === right.length && left.every((item, index) => item.id === right[index]?.id)
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

function resolveElementTarget(target: EventTarget | null): Element | null {
  return target instanceof Element ? target : null
}

function findClosestInside(
  target: Element,
  selector: string,
  container: HTMLElement,
): HTMLElement | null {
  const matched = target.closest(selector)
  if (!(matched instanceof HTMLElement)) return null
  if (!container.contains(matched)) return null
  return matched
}

export function useSortableMediaInputs<T extends SortableMediaInput>({
  containerRef,
  sourceItems,
  draggableSelector,
  filterSelector,
  onUpdate,
}: UseSortableMediaInputsOptions<T>) {
  const sortableItems = ref<T[]>([]) as Ref<T[]>
  const isReordering = ref(false)
  // Cached draggable list: populated at drag start so per-pointermove hover
  // resolution avoids querySelectorAll (forced reflow in the hot path).
  // Refreshed on commit only.
  let cachedDraggableElements: HTMLElement[] | null = null
  let cachedDraggableContainer: HTMLElement | null = null
  let cachedDraggableIndexByElement: Map<HTMLElement, number> | null = null

  function getSourceItems(): T[] {
    return [...toValue(sourceItems)]
  }

  function refreshDraggableCache(container: HTMLElement) {
    cachedDraggableContainer = container
    cachedDraggableElements = Array.from(container.querySelectorAll<HTMLElement>(draggableSelector))
    cachedDraggableIndexByElement = new Map(
      cachedDraggableElements.map((element, index) => [element, index] as const),
    )
  }

  function invalidateDraggableCache() {
    cachedDraggableElements = null
    cachedDraggableContainer = null
    cachedDraggableIndexByElement = null
  }

  function finishReorder() {
    invalidateDraggableCache()
    void nextTick(() => {
      isReordering.value = false
      sortableItems.value = getSourceItems()
    })
  }

  function clearHoverTarget(event: SortableReorderEvent) {
    event.hoverElement?.classList.remove('is-sortable-ghost')
    event.hoverElement = null
    event.hoverItemId = null
  }

  function clearDragClasses(event: SortableReorderEvent) {
    event.element.classList.remove('is-sortable-chosen', 'is-sortable-drag')
    clearHoverTarget(event)
  }

  function activateReorder(event: SortableReorderEvent) {
    if (event.isActive) return
    event.isActive = true
    isReordering.value = true
    event.element.classList.add('is-sortable-chosen', 'is-sortable-drag')
  }

  function getDraggableElements(container: HTMLElement): HTMLElement[] {
    if (cachedDraggableContainer === container && cachedDraggableElements) {
      return cachedDraggableElements
    }
    return Array.from(container.querySelectorAll<HTMLElement>(draggableSelector))
  }

  function resolveDraggableIndex(container: HTMLElement, element: HTMLElement): number | null {
    if (cachedDraggableContainer === container && cachedDraggableIndexByElement) {
      const cached = cachedDraggableIndexByElement.get(element)
      return cached === undefined ? null : cached
    }
    const index = getDraggableElements(container).indexOf(element)
    return index >= 0 ? index : null
  }

  function resolveItemIdAtIndex(index: number | null): string | null {
    if (index === null) return null
    return sortableItems.value[index]?.id ?? null
  }

  function resolveHoveredDraggable(event: PointerEvent): HTMLElement | null {
    const container = containerRef.value
    if (!container) return null
    const hitTarget = document.elementFromPoint?.(event.clientX, event.clientY)
    if (!hitTarget) return null
    return findClosestInside(hitTarget, draggableSelector, container)
  }

  function updateHoverTarget(event: PointerEvent, reorderEvent: SortableReorderEvent) {
    const container = containerRef.value
    const nextHoverElement = container ? resolveHoveredDraggable(event) : null
    const nextHoverIndex =
      container && nextHoverElement ? resolveDraggableIndex(container, nextHoverElement) : null
    const nextHoverItemId = resolveItemIdAtIndex(nextHoverIndex)

    if (nextHoverItemId === reorderEvent.hoverItemId) return
    clearHoverTarget(reorderEvent)
    if (!nextHoverElement || !nextHoverItemId || nextHoverItemId === reorderEvent.itemId) return

    nextHoverElement.classList.add('is-sortable-ghost')
    reorderEvent.hoverElement = nextHoverElement
    reorderEvent.hoverItemId = nextHoverItemId
  }

  function updateHoverTargetIfPointerIsOverDraggable(
    event: PointerEvent,
    reorderEvent: SortableReorderEvent,
  ) {
    if (!resolveHoveredDraggable(event)) return
    updateHoverTarget(event, reorderEvent)
  }

  function commitSortableUpdate(event: SortableReorderEvent) {
    const currentItems = getSourceItems()
    const from = currentItems.findIndex((item) => item.id === event.itemId)
    const to = event.hoverItemId
      ? currentItems.findIndex((item) => item.id === event.hoverItemId)
      : -1
    if (from < 0 || to < 0) {
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

  const pointerDragSession = usePointerDragSession<SortableReorderEvent>({
    onMove({ event, payload }) {
      if (!payload.isActive) {
        const deltaX = event.clientX - payload.startX
        const deltaY = event.clientY - payload.startY
        if (Math.hypot(deltaX, deltaY) < SORTABLE_DRAG_TOLERANCE_PX) return
        activateReorder(payload)
      }
      updateHoverTarget(event, payload)
    },
    onEnd({ event, payload }) {
      if (payload.isActive) {
        updateHoverTargetIfPointerIsOverDraggable(event, payload)
        commitSortableUpdate(payload)
      }
      clearDragClasses(payload)
      finishReorder()
    },
    onCancel({ payload }) {
      clearDragClasses(payload)
      finishReorder()
    },
  })

  function shouldIgnorePointerDown(event: PointerEvent) {
    return event.button !== 0 || event.isPrimary === false
  }

  function onContainerPointerDown(event: PointerEvent) {
    if (shouldIgnorePointerDown(event)) return
    const container = containerRef.value
    const target = resolveElementTarget(event.target)
    if (!container || !target) return

    if (filterSelector && findClosestInside(target, filterSelector, container)) return

    const draggableElement = findClosestInside(target, draggableSelector, container)
    if (!draggableElement) return

    // Snapshot the draggable list once per drag; hover resolution during the
    // drag reuses this cache instead of querying the DOM per pointermove.
    refreshDraggableCache(container)
    const itemId = resolveItemIdAtIndex(resolveDraggableIndex(container, draggableElement))
    if (!itemId) {
      invalidateDraggableCache()
      return
    }

    event.preventDefault()
    pointerDragSession.beginDrag(
      event,
      {
        itemId,
        element: draggableElement,
        startX: event.clientX,
        startY: event.clientY,
        isActive: false,
        hoverElement: null,
        hoverItemId: null,
      },
      { target: draggableElement },
    )
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

  watch(
    containerRef,
    (container, _previousContainer, onCleanup) => {
      if (!container) return
      container.addEventListener('pointerdown', onContainerPointerDown)
      onCleanup(() => {
        container.removeEventListener('pointerdown', onContainerPointerDown)
      })
    },
    { immediate: true },
  )

  return {
    isReordering,
    sortableItems,
  }
}
