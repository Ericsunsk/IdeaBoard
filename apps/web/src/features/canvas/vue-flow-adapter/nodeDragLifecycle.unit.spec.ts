import type { NodeChange } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import { createCanvasGraphStore } from '../graph-runtime'
import { createCanvasNodeDragLifecycle } from './nodeDragLifecycle'

function createTerminalPositionChange(nodeId: string): NodeChange {
  return {
    id: nodeId,
    type: 'position',
    dragging: false,
  } as unknown as NodeChange
}

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
    {
      id: 2,
      type: 'prompt',
      x: 800,
      y: 40,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  const store = createCanvasGraphStore(snapshot)
  const committedMembershipChanges: NodeChange[][] = []
  const lifecycleOrder: string[] = []

  const lifecycle = createCanvasNodeDragLifecycle({
    createCommand: () => ({ store, transaction: store.beginTransaction('node-drag') }),
    onBegin: () => lifecycleOrder.push('begin'),
    reconcileMembership: (committedStore, transaction, changes) => {
      expect(committedStore).toBe(store)
      expect(transaction).toBeDefined()
      committedMembershipChanges.push(changes)
      lifecycleOrder.push('membership-stage')
      return () => lifecycleOrder.push(`membership:${store.getNode('prompt-1')?.snapshot.x}`)
    },
  })

  return {
    lifecycle,
    store,
    committedMembershipChanges,
    lifecycleOrder,
  }
}

describe('createCanvasNodeDragLifecycle', () => {
  it('commits one coalesced drag through graph, history, dirty, and membership boundaries', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
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
    ).toMatchObject({
      consumed: [expect.objectContaining({ id: 'prompt-1', type: 'position' })],
      remaining: [],
    })
    expect(
      harness.lifecycle.handleChanges([
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 620, y: 96 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ]),
    ).toMatchObject({
      consumed: [expect.objectContaining({ id: 'prompt-1', type: 'position' })],
      remaining: [],
    })

    expect(harness.lifecycle.isActive()).toBe(true)
    harness.lifecycle.commit()

    expect(harness.committedMembershipChanges).toEqual([
      [
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 620, y: 96 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ],
    ])
    expect(harness.store.getNode('prompt-1')).toMatchObject({
      selected: false,
      snapshot: { x: 620, y: 96 },
    })
    expect(harness.lifecycleOrder).toEqual(['begin', 'membership-stage', 'membership:620'])
    expect(harness.lifecycle.isActive()).toBe(false)
  })

  it('cancels staged drag work without committing it', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
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

    expect(harness.store.getNode('prompt-1')?.snapshot).toMatchObject({ x: 40, y: 40 })
    expect(harness.committedMembershipChanges).toEqual([])
    expect(harness.lifecycleOrder).toEqual(['begin'])
    expect(harness.lifecycle.isActive()).toBe(false)
  })

  it('retains the last position when Vue Flow ends a drag without one', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
    harness.lifecycle.handleChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 620, y: 96 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])
    harness.lifecycle.handleChanges([createTerminalPositionChange('prompt-1')])

    harness.lifecycle.commit()

    expect(harness.store.getNode('prompt-1')).toMatchObject({
      snapshot: { x: 620, y: 96 },
    })
    expect(harness.committedMembershipChanges).toEqual([
      [
        expect.objectContaining({
          id: 'prompt-1',
          position: { x: 620, y: 96 },
          dragging: false,
        }),
      ],
    ])
  })

  it('keeps non-target position changes in the active graph transaction', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
    expect(
      harness.lifecycle.handleChanges([
        {
          id: 'prompt-2',
          type: 'position',
          position: { x: 900, y: 120 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 160, y: 120 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ]),
    ).toMatchObject({
      consumed: [
        expect.objectContaining({ id: 'prompt-2', type: 'position' }),
        expect.objectContaining({ id: 'prompt-1', type: 'position' }),
      ],
      remaining: [],
    })

    harness.lifecycle.commit()

    expect(harness.committedMembershipChanges[0]).toEqual([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 160, y: 120 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])
    expect(harness.store.getNode('prompt-2')).toMatchObject({
      snapshot: { x: 900, y: 120 },
    })
  })

  it('does not consume non-position renderer changes while a drag is active', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])

    expect(
      harness.lifecycle.handleChanges([
        {
          id: 'prompt-2',
          type: 'select',
          selected: true,
        },
      ]),
    ).toBeNull()

    harness.lifecycle.cancel()
    expect(harness.lifecycle.isActive()).toBe(false)
  })

  it('allows a renderer boundary to atomically stage final selection with the drag', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
    harness.lifecycle.handleChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 160, y: 120 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])

    harness.lifecycle.commit((store, transaction) => {
      store.getNodeIds().forEach((nodeId) => transaction.selectNode(nodeId, nodeId === 'prompt-1'))
    })

    expect(harness.store.getNode('prompt-1')).toMatchObject({
      selected: true,
      snapshot: { x: 160, y: 120 },
    })
  })

  it('rebases drag work when another graph commit lands during the gesture', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
    harness.lifecycle.handleChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 160, y: 120 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])

    harness.store.beginTransaction('node-update').updateNode('prompt-2', { x: 920 }).commit()

    expect(() =>
      harness.lifecycle.handleChanges([
        {
          id: 'prompt-1',
          type: 'position',
          position: { x: 220, y: 140 },
          from: { x: 40, y: 40 },
          dragging: true,
        },
      ]),
    ).not.toThrow()

    expect(() => harness.lifecycle.commit()).not.toThrow()
    expect(harness.store.getNode('prompt-1')).toMatchObject({ snapshot: { x: 220, y: 140 } })
    expect(harness.store.getNode('prompt-2')).toMatchObject({ snapshot: { x: 920 } })
  })

  it('rebases final position and selection when the transaction becomes stale before drag stop', () => {
    const harness = createHarness()

    harness.lifecycle.begin(['prompt-1'])
    harness.lifecycle.handleChanges([
      {
        id: 'prompt-1',
        type: 'position',
        position: { x: 260, y: 160 },
        from: { x: 40, y: 40 },
        dragging: true,
      },
    ])

    harness.store.beginTransaction('node-update').updateNode('prompt-2', { text: 'ready' }).commit()

    expect(() =>
      harness.lifecycle.commit((store, transaction) => {
        store
          .getNodeIds()
          .forEach((nodeId) => transaction.selectNode(nodeId, nodeId === 'prompt-1'))
      }),
    ).not.toThrow()
    expect(harness.store.getNode('prompt-1')).toMatchObject({
      selected: true,
      snapshot: { x: 260, y: 160 },
    })
    expect(harness.store.getNode('prompt-2')).toMatchObject({ snapshot: { text: 'ready' } })
  })
})
