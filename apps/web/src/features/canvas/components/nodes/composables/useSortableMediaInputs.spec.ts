import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSortableMediaInputs } from './useSortableMediaInputs'

type CapturedSortableOptions = {
  onStart?: (event: unknown) => void
  onUpdate?: (event: {
    oldDraggableIndex?: number
    newDraggableIndex?: number
    oldIndex?: number
    newIndex?: number
  }) => void
  onEnd?: (event: unknown) => void
}

const sortableMock = vi.hoisted(() => ({
  options: null as CapturedSortableOptions | null,
}))

vi.mock('@vueuse/integrations/useSortable', () => ({
  useSortable: vi.fn((_el: unknown, _list: unknown, options: CapturedSortableOptions) => {
    sortableMock.options = options
    return {
      option: vi.fn(),
      start: vi.fn(),
      stop: vi.fn(),
    }
  }),
}))

type Item = { id: string }

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
          fallbackClass: 'fallback',
          onUpdate,
        })
        return () => h('div', { ref: containerRef })
      },
    }),
  )
  app.mount(host)

  return {
    app,
    host,
    onUpdate,
    sourceItems,
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
    sortableMock.options = null
  })

  it('commits reorder from draggable indexes without reading DOM dataset order', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      sortableMock.options?.onStart?.({})
      sortableMock.options?.onUpdate?.({
        oldDraggableIndex: 0,
        newDraggableIndex: 2,
      })
      sortableMock.options?.onEnd?.({})

      expect(harness.onUpdate).toHaveBeenCalledWith([{ id: 'b' }, { id: 'c' }, { id: 'a' }])

      await nextTick()
      expect(harness.state.isReordering.value).toBe(false)
      expect(harness.state.sortableItems.value).toEqual([{ id: 'b' }, { id: 'c' }, { id: 'a' }])
    } finally {
      harness.unmount()
    }
  })

  it('uses oldIndex/newIndex fallback when draggable indexes are unavailable', async () => {
    const harness = mountSortableHarness([{ id: 'a' }, { id: 'b' }, { id: 'c' }])
    await nextTick()

    try {
      sortableMock.options?.onStart?.({})
      sortableMock.options?.onUpdate?.({
        oldIndex: 2,
        newIndex: 0,
      })
      sortableMock.options?.onEnd?.({})

      expect(harness.onUpdate).toHaveBeenCalledWith([{ id: 'c' }, { id: 'a' }, { id: 'b' }])
    } finally {
      harness.unmount()
    }
  })
})
