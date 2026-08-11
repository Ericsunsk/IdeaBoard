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
  it('owns typed execution state through an attachment-scoped lease', () => {
    const runtime = createCanvasExecutionRuntime()
    const attachment = runtime.attachBoard('board-a')
    const lease = runtime.startNode(attachment, 'image-2', { status: 'submitting' })

    expect(lease).not.toBeNull()
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'submitting' })
    expect(runtime.isNodeActive('image-2')).toBe(true)

    expect(runtime.transitionNode(lease!, { status: 'running', taskId: ' task-1 ' })).toBe(true)
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'running', taskId: 'task-1' })

    expect(runtime.transitionNode(lease!, { status: 'succeeded', taskId: 'task-1' })).toBe(true)
    expect(runtime.isNodeActive('image-2')).toBe(false)
  })

  it('invalidates an old attachment even when the same Board is reopened', () => {
    const runtime = createCanvasExecutionRuntime()
    const firstAttachment = runtime.attachBoard('board-a')
    const oldLease = runtime.startNode(firstAttachment, 'image-2', { status: 'running' })!
    const nextAttachment = runtime.attachBoard('board-a')
    const nextLease = runtime.startNode(nextAttachment, 'image-2', {
      status: 'running',
      taskId: 'task-new',
    })!

    expect(nextAttachment.epoch).toBeGreaterThan(firstAttachment.epoch)
    expect(runtime.transitionNode(oldLease, { status: 'cancelled' })).toBe(false)
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'running', taskId: 'task-new' })
    expect(runtime.transitionNode(nextLease, { status: 'succeeded', taskId: 'task-new' })).toBe(
      true,
    )
  })

  it('uses lease CAS so an old execution cannot overwrite a newer execution on the same node', () => {
    const runtime = createCanvasExecutionRuntime()
    const attachment = runtime.attachBoard('board-a')
    const oldLease = runtime.startNode(attachment, 'image-2', {
      status: 'running',
      taskId: 'task-old',
    })!
    const nextLease = runtime.startNode(attachment, 'image-2', {
      status: 'running',
      taskId: 'task-new',
    })!

    expect(runtime.transitionNode(oldLease, { status: 'failed', taskId: 'task-old' })).toBe(false)
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'running', taskId: 'task-new' })
    expect(runtime.transitionNode(nextLease, { status: 'succeeded', taskId: 'task-new' })).toBe(
      true,
    )
  })

  it('publishes immutable map replacements and rejects blank identities', () => {
    const runtime = createCanvasExecutionRuntime()
    const initialStates = runtime.nodeStates.value

    expect(() => runtime.attachBoard('  ')).toThrow('Canvas execution boardId is required')

    const attachment = runtime.attachBoard('board-a')
    expect(() => runtime.startNode(attachment, '  ', { status: 'submitting' })).toThrow(
      'Canvas execution nodeId is required',
    )
    const lease = runtime.startNode(attachment, 'image-2', { status: 'running' })!

    expect(runtime.nodeStates.value).not.toBe(initialStates)
    expect(initialStates.size).toBe(0)
    expect(runtime.clearNode(lease)).toBe(true)
    expect(runtime.nodeStates.value.size).toBe(0)
  })
})

describe('projectCanvasExecutionEdgeAnimation', () => {
  it.each(['submitting', 'running'] as const)(
    'animates image input edges while the target node is %s',
    (status) => {
      const runtime = createCanvasExecutionRuntime()
      const attachment = runtime.attachBoard('board-a')
      runtime.startNode(attachment, 'image-2', { status })

      const result = projectCanvasExecutionEdgeAnimation([makeEdge()], runtime.nodeStates.value)

      expect(result.didChange).toBe(true)
      expect(result.edges).toEqual([expect.objectContaining({ animated: true })])
    },
  )

  it.each(['succeeded', 'failed', 'cancelled'] as const)(
    'stops image input edge animation when the target node is %s',
    (status) => {
      const runtime = createCanvasExecutionRuntime()
      const attachment = runtime.attachBoard('board-a')
      runtime.startNode(attachment, 'image-2', { status, taskId: 'task-1' })

      const result = projectCanvasExecutionEdgeAnimation(
        [makeEdge({ animated: true })],
        runtime.nodeStates.value,
      )

      expect(result.didChange).toBe(true)
      expect(result.edges).toEqual([expect.objectContaining({ animated: false })])
    },
  )

  it('clears stale animation and preserves non-image edge identity', () => {
    const runtime = createCanvasExecutionRuntime()
    runtime.attachBoard('board-a')
    const imageEdge = makeEdge({ animated: true })
    const promptEdge = makeEdge({
      id: 'prompt-edge',
      targetHandle: 'in:prompt',
      animated: true,
    })

    const result = projectCanvasExecutionEdgeAnimation(
      [imageEdge, promptEdge],
      runtime.nodeStates.value,
    )

    expect(result.edges[0]).toEqual(expect.objectContaining({ animated: false }))
    expect(result.edges[1]).toBe(promptEdge)
  })
})
