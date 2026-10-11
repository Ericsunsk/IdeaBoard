import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useSortableMediaInputs } from './useSortableMediaInputs'

type Item = { id: string }

const elementFromPointMock = vi.fn<() => Element | null>()
let originalElementFromPoint: Document['elementFromPoint']

function createPointerEvent(type: string, init: Partial<PointerEventInit> = {}) {
  const event = new Event(type, { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperties(event, {
    button: { value: init.button ?? 0 },
    clientX: { value: init.clientX ?? 0 },
    clientY: { value: init.clientY ?? 0 },
    isPrimary: { value: init.isPrimary ?? true },
    pointerId: { value: init.pointerId ?? 1 },
  })
  return event
}

function mountSortableHarness(initialItems: Item[]) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const sourceItems = ref<readonly Item[]>(initialItems)
  const containerRef = ref<HTMLElement | null>(null)
  const onUpdate = vi.fn((items: Item[]) => {
    sourceItems.value = items
  })
  let state: ReturnType<typeof useSortableMediaInputs<Item>>

  const app = createApp(
    defineComponent({
      setup() {
        state = useSortableMediaInputs({
          containerRef,
          sourceItems,
          draggableSelector: '.item',
          filterSelector: '.filtered',
          onUpdate,
        })
        return () =>
          h(
            'div',
            { ref: containerRef },
            state.sortableItems.value.map((item) =>
              h('div', { class: 'item', 'data-id': item.id }, [
                h('span', item.id),
                h('button', { class: 'filtered', type: 'button' }, 'remove'),
              ]),
            ),
          )
      },
    }),
  )
  app.mount(host)

  return {
    app,
    host,
    onUpdate,
    sourceItems,
    get items() {
      return Array.from(host.querySelectorAll<HTMLElement>('.item'))
    },
    get removeButtons() {
      return Array.from(host.querySelectorAll<HTMLElement>('.filtered'))
    },
    get state() {
      return state
    },
    unmount() {
      app.unmount()
      host.remove()
    },
  }
}

describe('useSortableMediaInputs', () => {
  beforeEach(() => {
    originalElementFromPoint = document.elementFromPoint
    elementFromPointMock.mockReset()
    Object.defineProperty(document, 'elementFromPoint', {
      configurable: true,
      value: elementFromPointMock,
    })
  })

  afterEach(() => {
    Object.defineProperty(document, 'elementFromPoint', {
      configurable: true,
      value: originalElementFromPoint,
    })
    vi.restoreAllMocks()
  })

  it('commits reorder when a dragged thumbnail is released over another draggable item', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.items[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      expect(harness.state.isReordering.value).toBe(false)

      elementFromPointMock.mockReturnValue(harness.items[2] ?? null)
      window.dispatchEvent(createPointerEvent('pointermove', { clientX: 8 }))
      expect(harness.state.isReordering.value).toBe(true)
      expect(harness.items[0]?.classList.contains('is-sortable-chosen')).toBe(true)
      expect(harness.items[0]?.classList.contains('is-sortable-drag')).toBe(true)

      expect(harness.items[2]?.classList.contains('is-sortable-ghost')).toBe(true)

      window.dispatchEvent(createPointerEvent('pointerup'))

      expect(harness.onUpdate).toHaveBeenCalledWith([{ id: 'b' }, { id: 'c' }, { id: 'a' }])

      await nextTick()
      expect(harness.state.isReordering.value).toBe(false)
      expect(harness.state.sortableItems.value).toEqual([{ id: 'b' }, { id: 'c' }, { id: 'a' }])
    } finally {
      harness.unmount()
    }
  })

  it('does not reorder on a click or sub-threshold pointer move', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.items[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      elementFromPointMock.mockReturnValue(harness.items[2] ?? null)
      window.dispatchEvent(createPointerEvent('pointermove', { clientX: 3, clientY: 0 }))
      window.dispatchEvent(createPointerEvent('pointerup', { clientX: 3, clientY: 0 }))

      expect(harness.state.isReordering.value).toBe(false)
      expect(harness.items[0]?.classList.contains('is-sortable-chosen')).toBe(false)
      expect(harness.onUpdate).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('commits to the last hovered item when pointerup lands in empty space', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.items[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      elementFromPointMock.mockReturnValue(harness.items[2] ?? null)
      window.dispatchEvent(createPointerEvent('pointermove', { clientX: 8 }))

      elementFromPointMock.mockReturnValue(null)
      window.dispatchEvent(createPointerEvent('pointerup', { clientX: 20 }))

      expect(harness.onUpdate).toHaveBeenCalledWith([{ id: 'b' }, { id: 'c' }, { id: 'a' }])
    } finally {
      harness.unmount()
    }
  })

  it('does not start reorder from filtered or non-draggable elements', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.removeButtons[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      expect(harness.state.isReordering.value).toBe(false)

      harness.host.dispatchEvent(createPointerEvent('pointerdown'))
      expect(harness.state.isReordering.value).toBe(false)

      elementFromPointMock.mockReturnValue(harness.items[2] ?? null)
      window.dispatchEvent(createPointerEvent('pointerup'))
      expect(harness.onUpdate).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('keeps source items as the source of truth around a cancelled reorder', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.sourceItems.value = [{ id: 'b' }, { id: 'a' }, { id: 'c' }]
      await nextTick()
      expect(harness.state.sortableItems.value).toEqual([{ id: 'b' }, { id: 'a' }, { id: 'c' }])

      harness.items[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      window.dispatchEvent(createPointerEvent('pointermove', { clientX: 8 }))
      harness.sourceItems.value = [{ id: 'c' }, { id: 'b' }, { id: 'a' }]
      await nextTick()
      expect(harness.state.sortableItems.value).toEqual([{ id: 'b' }, { id: 'a' }, { id: 'c' }])

      elementFromPointMock.mockReturnValue(null)
      window.dispatchEvent(createPointerEvent('pointerup'))
      expect(harness.onUpdate).not.toHaveBeenCalled()

      await nextTick()
      expect(harness.state.isReordering.value).toBe(false)
      expect(harness.state.sortableItems.value).toEqual([{ id: 'c' }, { id: 'b' }, { id: 'a' }])
    } finally {
      harness.unmount()
    }
  })

  it('cleans up active pointer listeners on unmount', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      harness.items[0]?.dispatchEvent(createPointerEvent('pointerdown'))
      window.dispatchEvent(createPointerEvent('pointermove', { clientX: 8 }))
      harness.unmount()

      elementFromPointMock.mockReturnValue(harness.items[2] ?? null)
      window.dispatchEvent(createPointerEvent('pointermove'))
      window.dispatchEvent(createPointerEvent('pointerup'))

      expect(harness.onUpdate).not.toHaveBeenCalled()
    } finally {
      if (document.body.contains(harness.host)) harness.unmount()
    }
  })
})
