import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../../canvas-board/domain'
import { CanvasGraphSession } from '..'
import { createGroupResizeRuntime } from './groupResizeRuntime'
import type { CanvasGroupFrame, CanvasRect } from '../../utils/canvasGroupGeometry'

describe('groupResizeRuntime', () => {
  function createHarness() {
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.groups = [
      {
        id: 'group-1',
        nodeIds: [],
        x: 10,
        y: 10,
        width: 100,
        height: 100,
      },
    ]
    const session = CanvasGraphSession.hydrate('board-test', snapshot)
    const previewBounds = ref<Map<string, CanvasRect>>(new Map())
    const groupFrames = computed<CanvasGroupFrame[]>(() => [
      {
        id: 'group-1',
        nodeIds: [],
        bounds: { left: 10, top: 10, right: 110, bottom: 110, width: 100, height: 100 },
        selected: true,
        order: 0,
      },
    ])

    const onWarning = vi.fn()
    const runtime = createGroupResizeRuntime({
      groupFrames,
      readCanvasGraphStore: () => session.store,
      isDragActive: () => false,
      clearPendingGroupJoin: vi.fn(() => false),
      selectGroup: vi.fn(() => true),
      setPreviewGroupBounds: (id, bounds) => {
        previewBounds.value.set(id, bounds)
      },
      clearPreviewGroupBounds: (id) => {
        previewBounds.value.delete(id)
      },
      onWarning,
      boundsUpdateFailedMessage: 'Update failed',
    })

    return { runtime, session, previewBounds, onWarning }
  }

  it('starts inactive and can begin resizing a valid group', () => {
    const { runtime } = createHarness()
    expect(runtime.isActive()).toBe(false)

    const began = runtime.begin('group-1', 'se', { x: 110, y: 110 })
    expect(began).toBe(true)
    expect(runtime.isActive()).toBe(true)
  })

  it('updates preview bounds on move and cancels cleanly', () => {
    const { runtime, previewBounds } = createHarness()
    runtime.begin('group-1', 'se', { x: 110, y: 110 })

    runtime.move({ x: 130, y: 130 })
    expect(previewBounds.value.get('group-1')).toMatchObject({
      width: 120,
      height: 120,
    })

    runtime.cancel()
    expect(runtime.isActive()).toBe(false)
    expect(previewBounds.value.has('group-1')).toBe(false)
  })

  it('commits resized geometry to store', () => {
    const { runtime, session } = createHarness()
    runtime.begin('group-1', 'se', { x: 110, y: 110 })
    runtime.move({ x: 160, y: 160 })

    const committed = runtime.commit()
    expect(committed).toBe(true)
    expect(runtime.isActive()).toBe(false)

    const group = session.store.getGroup('group-1')
    expect(group).toMatchObject({
      width: 150,
      height: 150,
    })
  })
})
