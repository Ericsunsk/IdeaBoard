import type { NodeChange } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import { createCanvasGraphStore } from '../graph-runtime'
import { createCanvasNodeDragLifecycle } from './nodeDragLifecycle'

function createHarness() {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 40,
      y: 40,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  const store = createCanvasGraphStore(snapshot)
  const scheduledFrames = new Map<number, FrameRequestCallback>()
  const projectedChanges: NodeChange[][] = []
  const syncedChanges: NodeChange[][] = []
  const committedMembershipChanges: NodeChange[][] = []
  const lifecycleOrder: string[] = []
  let nextFrameId = 1

  const lifecycle = createCanvasNodeDragLifecycle({
    createCommand: () => ({ store, transaction: store.beginTransaction('node-drag') }),
    projectChanges: (changes) => projectedChanges.push(changes),
    syncChanges: (changes) => syncedChanges.push(changes),
    captureHistory: () => {
      lifecycleOrder.push('history-capture')
      return { token: 'before' }
    },
    commitHistory: (before) => {
      expect(before).toEqual({ token: 'before' })
      lifecycleOrder.push('history-commit')
      return true
    },
    advanceLocalEditVersion: () => lifecycleOrder.push('local-edit'),
    markDirtyAndScheduleAutosave: () => lifecycleOrder.push('dirty'),
    onBegin: () => lifecycleOrder.push('begin'),
    reconcileMembership: (committedStore, changes) => {
      expect(committedStore).toBe(store)
      committedMembershipChanges.push(changes)
      lifecycleOrder.push(`membership:${store.getNode('prompt-1')?.snapshot.x}`)
    },
    scheduleAnimationFrame: (callback) => {
      const frameId = nextFrameId++
      scheduledFrames.set(frameId, callback)
      return frameId
    },
    cancelAnimationFrame: (frameId) => {
      scheduledFrames.delete(frameId)
    },
  })

  return {
    lifecycle,
    store,
    projectedChanges,
    syncedChanges,
    committedMembershipChanges,
    lifecycleOrder,
    flushFrames() {
      const callbacks = [...scheduledFrames.values()]
      scheduledFrames.clear()
      callbacks.forEach((callback) => callback(performance.now()))
    },
  }
}

describe('createCanvasNodeDragLifecycle', () => {
  it('commits one coalesced drag through graph, history, dirty, and membership boundaries', () => {
    const harness = createHarness()

    harness.lifecycle.begin()
    expect(
      harness.lifecycle.handleChanges([
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 440, y: 80 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ]),
    ).toBe(true)
    expect(
      harness.lifecycle.handleChanges([
        { id: 'prompt-1', type: 'select', selected: true },
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 620, y: 96 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ]),
    ).toBe(true)

    expect(harness.lifecycle.isActive()).toBe(true)
    expect(harness.projectedChanges).toEqual([])

    harness.flushFrames()
    harness.lifecycle.commit()

    expect(harness.projectedChanges).toHaveLength(1)
    expect(harness.syncedChanges).toHaveLength(1)
    expect(harness.syncedChanges[0]).toEqual([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 96 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
      { id: 'prompt-1', type: 'select', selected: true },
    ])
    expect(harness.store.getNode('prompt-1')).toMatchObject({
      selected: true,
      snapshot: { x: 620, y: 96 },
    })
    expect(harness.committedMembershipChanges).toEqual([harness.syncedChanges[0]])
    expect(harness.lifecycleOrder).toEqual([
      'begin',
      'history-capture',
      'local-edit',
      'membership:620',
      'history-commit',
      'dirty',
    ])
    expect(harness.lifecycle.isActive()).toBe(false)
  })

  it('cancels queued drag work without projection or commit side effects', () => {
    const harness = createHarness()

    harness.lifecycle.begin()
    harness.lifecycle.handleChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 720, y: 96 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])
    harness.lifecycle.cancel()
    harness.flushFrames()

    expect(harness.store.getNode('prompt-1')?.snapshot).toMatchObject({ x: 40, y: 40 })
    expect(harness.projectedChanges).toEqual([])
    expect(harness.syncedChanges).toEqual([])
    expect(harness.committedMembershipChanges).toEqual([])
    expect(harness.lifecycleOrder).toEqual(['begin', 'history-capture', 'local-edit'])
    expect(harness.lifecycle.isActive()).toBe(false)
  })
})
