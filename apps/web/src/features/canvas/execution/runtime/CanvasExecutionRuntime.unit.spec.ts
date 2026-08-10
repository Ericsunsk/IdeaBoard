import type { Edge } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { createCanvasExecutionRuntime, projectCanvasExecutionEdgeAnimation } from '../index'

function makeEdge(overrides: Partial<Edge> = {}): Edge {
  return {
    id: 'edge-1',
    source: 'image-1',
    sourceHandle: 'out:image',
    target: 'image-2',
    targetHandle: 'in:image',
    animated: false,
    ...overrides,
  }
}

describe('CanvasExecutionRuntime', () => {
  it('owns typed execution state transitions per node with task identity when available', () => {
    const runtime = createCanvasExecutionRuntime()

    expect(runtime.getNodeState('image-2')).toBeNull()

    runtime.transitionNode('image-2', { status: 'submitting' })
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'submitting' })
    expect(runtime.isNodeActive('image-2')).toBe(true)

    runtime.transitionNode('image-2', { status: 'running', taskId: ' task-1 ' })
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'running', taskId: 'task-1' })

    for (const status of ['succeeded', 'failed', 'cancelled'] as const) {
      runtime.transitionNode('image-2', { status, taskId: 'task-1' })
      expect(runtime.getNodeState('image-2')).toEqual({ status, taskId: 'task-1' })
      expect(runtime.isNodeActive('image-2')).toBe(false)
    }
  })

  it('publishes immutable map replacements and supports explicit state cleanup', () => {
    const runtime = createCanvasExecutionRuntime()
    const initialStates = runtime.nodeStates.value

    runtime.transitionNode('image-2', { status: 'running', taskId: 'task-1' })

    expect(runtime.nodeStates.value).not.toBe(initialStates)
    expect(initialStates.size).toBe(0)
    expect(runtime.nodeStates.value.get('image-2')).toEqual({
      status: 'running',
      taskId: 'task-1',
    })

    runtime.clearNode('image-2')
    expect(runtime.nodeStates.value.size).toBe(0)
  })

  it('rejects blank node identities at the runtime boundary', () => {
    const runtime = createCanvasExecutionRuntime()

    expect(() => runtime.transitionNode('  ', { status: 'submitting' })).toThrow(
      'Canvas execution nodeId is required',
    )
  })
})

describe('projectCanvasExecutionEdgeAnimation', () => {
  it.each(['submitting', 'running'] as const)(
    'animates image input edges while the target node is %s',
    (status) => {
      const runtime = createCanvasExecutionRuntime()
      runtime.transitionNode('image-2', { status })

      const result = projectCanvasExecutionEdgeAnimation([makeEdge()], runtime.nodeStates.value)

      expect(result.didChange).toBe(true)
      expect(result.edges).toEqual([expect.objectContaining({ animated: true })])
    },
  )

  it.each(['succeeded', 'failed', 'cancelled'] as const)(
    'stops image input edge animation when the target node is %s',
    (status) => {
      const runtime = createCanvasExecutionRuntime()
      runtime.transitionNode('image-2', { status, taskId: 'task-1' })

      const result = projectCanvasExecutionEdgeAnimation(
        [makeEdge({ animated: true })],
        runtime.nodeStates.value,
      )

      expect(result.didChange).toBe(true)
      expect(result.edges).toEqual([expect.objectContaining({ animated: false })])
    },
  )

  it('clears stale image input animation when the target has no execution state', () => {
    const runtime = createCanvasExecutionRuntime()

    const result = projectCanvasExecutionEdgeAnimation(
      [makeEdge({ animated: true })],
      runtime.nodeStates.value,
    )

    expect(result.didChange).toBe(true)
    expect(result.edges).toEqual([expect.objectContaining({ animated: false })])
  })

  it('preserves non-image edge animation and stable identities when projection is unchanged', () => {
    const runtime = createCanvasExecutionRuntime()
    runtime.transitionNode('fusion-1', { status: 'running' })
    const imageEdge = makeEdge({ id: 'image-edge' })
    const promptEdge = makeEdge({
      id: 'prompt-edge',
      target: 'fusion-1',
      targetHandle: 'in:prompt',
      animated: true,
    })
    const edges = [imageEdge, promptEdge]

    const result = projectCanvasExecutionEdgeAnimation(edges, runtime.nodeStates.value)

    expect(result.didChange).toBe(false)
    expect(result.edges).toBe(edges)
    expect(result.edges[0]).toBe(imageEdge)
    expect(result.edges[1]).toBe(promptEdge)
  })

  it('scopes an incremental projection to the execution node that changed', () => {
    const runtime = createCanvasExecutionRuntime()
    runtime.transitionNode('image-2', { status: 'succeeded', taskId: 'task-1' })
    const completedEdge = makeEdge({ id: 'completed-edge', target: 'image-2', animated: true })
    const legacyReattachEdge = makeEdge({
      id: 'reattach-edge',
      target: 'image-3',
      animated: true,
    })

    const result = projectCanvasExecutionEdgeAnimation(
      [completedEdge, legacyReattachEdge],
      runtime.nodeStates.value,
      ['image-2'],
    )

    expect(result.edges).toEqual([
      expect.objectContaining({ id: 'completed-edge', animated: false }),
      legacyReattachEdge,
    ])
  })
})
