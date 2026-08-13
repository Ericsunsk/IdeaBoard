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
    const lease = runtime.beginSubmission(attachment, 'image-2')

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
    const oldLease = runtime.hydratePending(firstAttachment, 'image-2', 'task-old')!
    const nextAttachment = runtime.attachBoard('board-a')
    const nextLease = runtime.hydratePending(nextAttachment, 'image-2', 'task-new')!

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
    const oldLease = runtime.hydratePending(attachment, 'image-2', 'task-old')!
    const nextLease = runtime.hydratePending(attachment, 'image-2', 'task-new')!

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
    expect(() => runtime.beginSubmission(attachment, '  ')).toThrow(
      'Canvas execution nodeId is required',
    )
    const lease = runtime.beginSubmission(attachment, 'image-2')!

    expect(runtime.nodeStates.value).not.toBe(initialStates)
    expect(initialStates.size).toBe(0)
    expect(runtime.cancelNodeExecutions([lease])).toEqual([])
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'cancelled' })
  })

  it('cancels only the current requested leases and returns their exact task ids', () => {
    const runtime = createCanvasExecutionRuntime()
    const attachment = runtime.attachBoard('board-a')
    const staleLease = runtime.hydratePending(attachment, 'image-2', 'task-stale')!
    const currentLease = runtime.hydratePending(attachment, 'image-2', 'task-current')!
    const terminalLease = runtime.hydratePending(attachment, 'image-3', 'task-terminal')!
    runtime.transitionNode(terminalLease, { status: 'succeeded', taskId: 'task-terminal' })

    expect(runtime.captureActiveNodeExecutions([' image-2 ', 'image-2', 'image-3'])).toEqual([
      currentLease,
    ])
    expect(
      runtime.cancelNodeExecutions([staleLease, currentLease, terminalLease, currentLease]),
    ).toEqual(['task-current'])
    expect(runtime.getNodeState('image-2')).toEqual({
      status: 'cancelled',
      taskId: 'task-current',
    })
    expect(runtime.getNodeState('image-3')).toEqual({
      status: 'succeeded',
      taskId: 'task-terminal',
    })
    expect(runtime.transitionNode(staleLease, { status: 'failed', taskId: 'task-stale' })).toBe(
      false,
    )
    expect(runtime.transitionNode(currentLease, { status: 'failed', taskId: 'task-current' })).toBe(
      false,
    )
  })

  it('keeps a shared task alive while another output slot remains active', () => {
    const runtime = createCanvasExecutionRuntime()
    const attachment = runtime.attachBoard('board-a')
    const firstLease = runtime.hydratePending(attachment, 'image-2', 'task-shared')!
    const secondLease = runtime.hydratePending(attachment, 'image-3', 'task-shared')!

    expect(runtime.cancelNodeExecutions([firstLease])).toEqual([])
    expect(runtime.getNodeState('image-2')).toEqual({ status: 'cancelled', taskId: 'task-shared' })
    expect(runtime.getNodeState('image-3')).toEqual({ status: 'running', taskId: 'task-shared' })

    expect(runtime.cancelNodeExecutions([secondLease])).toEqual(['task-shared'])
  })
})

describe('projectCanvasExecutionEdgeAnimation', () => {
  it.each(['submitting', 'running'] as const)(
    'animates image input edges while the target node is %s',
    (status) => {
      const runtime = createCanvasExecutionRuntime()
      const attachment = runtime.attachBoard('board-a')
      if (status === 'submitting') runtime.beginSubmission(attachment, 'image-2')
      else runtime.hydratePending(attachment, 'image-2', 'task-1')

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
      const lease = runtime.hydratePending(attachment, 'image-2', 'task-1')!
      runtime.transitionNode(lease, { status, taskId: 'task-1' })

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
