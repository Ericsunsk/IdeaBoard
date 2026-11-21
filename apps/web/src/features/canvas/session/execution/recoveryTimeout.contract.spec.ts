import { afterEach, describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board'
import { CanvasGraphSession } from '../board-session'
import {
  BoardGraphSession,
  createCanvasExecutionRecoveryCoordinator,
  createPendingImageExecutionCore,
  type CanvasExecutionRecoveryCoordinatorOptions,
} from '.'

type Observation = Awaited<ReturnType<CanvasExecutionRecoveryCoordinatorOptions['observeTask']>>

function createHarness() {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = [0, 1].map((resultIndex) => ({
    id: resultIndex + 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: 'task-shared',
    pendingResultIndex: resultIndex,
    pendingClientActionId: 'action-1',
    outputStatus: 'running',
    outputImages: [],
  }))
  const graph = CanvasGraphSession.hydrate('board-a', snapshot)
  const core = createPendingImageExecutionCore()
  const onMaterialized = vi.fn()
  core.attachBoard(BoardGraphSession.create('board-a', { graphSession: graph, onMaterialized }))
  const observeTask = vi.fn<CanvasExecutionRecoveryCoordinatorOptions['observeTask']>()
  const flushAutosave = vi.fn(async () => undefined)
  const cancelTasks = vi.fn()
  const onWarning = vi.fn()
  const onError = vi.fn(() => true)
  const coordinator = createCanvasExecutionRecoveryCoordinator({
    executionCore: core,
    readBoardSnapshot: () => graph.exportSnapshot(snapshot),
    observeTask,
    flushAutosave,
    cancelTasks,
    onWarning,
    onError,
  })
  return {
    graph,
    core,
    coordinator,
    observeTask,
    flushAutosave,
    cancelTasks,
    onWarning,
    onError,
    onMaterialized,
  }
}

function completedObservation() {
  return {
    resolve: vi.fn<Observation['resolve']>(async (resultIndex) => ({
      kind: 'completed',
      result: { url: `https://example.com/${resultIndex}.png`, ratio: 1 },
    })),
  }
}

async function flushPromises() {
  for (let turn = 0; turn < 30; turn += 1) await Promise.resolve()
}

afterEach(() => vi.useRealTimers())

describe('pending image recovery after foreground timeout', () => {
  it('returns the foreground result and automatically recovers shared outputs without another driver', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    let finishExpired!: (value: Observation) => void
    let finishBackground!: (value: Observation) => void
    harness.observeTask
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishExpired = resolve
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishBackground = resolve
          }),
      )

    const recovery = harness.coordinator.recover()
    expect(harness.coordinator.recover()).toBe(recovery)
    await vi.advanceTimersByTimeAsync(360_000)
    expect(await recovery).toMatchObject({ pendingNodeIds: ['image-1', 'image-2'] })
    expect(harness.observeTask).toHaveBeenCalledOnce()
    expect(harness.observeTask.mock.calls[0]?.[1].signal.aborted).toBe(true)
    expect(harness.graph.exportSnapshot().imageNodes.map((node) => node.pendingTaskId)).toEqual([
      'task-shared',
      'task-shared',
    ])

    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.observeTask).toHaveBeenCalledTimes(2)
    expect(harness.coordinator.recover()).toBe(recovery)
    expect(harness.coordinator.recover()).toBe(recovery)
    const observation = completedObservation()
    finishBackground(observation)
    await flushPromises()
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'succeeded',
    ])
    expect(observation.resolve.mock.calls.map(([index]) => index)).toEqual([0, 1])
    expect(harness.onMaterialized).toHaveBeenCalledTimes(2)
    expect(harness.flushAutosave).toHaveBeenCalledOnce()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.onWarning).not.toHaveBeenCalled()

    const expiredObservation = completedObservation()
    finishExpired(expiredObservation)
    await flushPromises()
    await harness.coordinator.recover()
    expect(expiredObservation.resolve).not.toHaveBeenCalled()
    expect(harness.observeTask).toHaveBeenCalledTimes(2)
    expect(harness.onMaterialized).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('bounds background observation, preserves pending facts, and allows another recovery on the same attachment', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    harness.observeTask.mockImplementation(() => new Promise(() => undefined))
    const first = harness.coordinator.recover()
    await vi.advanceTimersByTimeAsync(360_000)
    expect((await first)?.pendingNodeIds).toEqual(['image-1', 'image-2'])
    await vi.advanceTimersByTimeAsync(315_000)
    expect(harness.observeTask).toHaveBeenCalledTimes(2)
    expect(harness.onWarning).toHaveBeenCalledExactlyOnceWith(
      'This is taking longer than expected. Check back in a moment.',
    )
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'running',
      'running',
    ])
    expect(harness.graph.exportSnapshot().imageNodes.map((node) => node.pendingTaskId)).toEqual([
      'task-shared',
      'task-shared',
    ])
    expect(vi.getTimerCount()).toBe(0)

    harness.observeTask.mockResolvedValue(completedObservation())
    const next = harness.coordinator.recover()
    expect(next).not.toBe(first)
    expect((await next)?.recoveredNodeIds).toEqual(['image-1', 'image-2'])
    expect(harness.observeTask).toHaveBeenCalledTimes(3)
    expect(harness.flushAutosave).toHaveBeenCalledOnce()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
  })

  it('reuses the terminal observation and only continues the output whose display timed out', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    let waitingCalls = 0
    const observation: Observation = {
      resolve: vi.fn<Observation['resolve']>((index) => {
        if (index === 1 && waitingCalls++ === 0) return new Promise(() => undefined)
        return Promise.resolve({
          kind: 'completed',
          result: { url: `https://example.com/${index}.png`, ratio: 1 },
        })
      }),
    }
    harness.observeTask.mockResolvedValue(observation)
    const recovery = harness.coordinator.recover()
    await vi.advanceTimersByTimeAsync(360_000)
    expect(await recovery).toMatchObject({
      recoveredNodeIds: ['image-1'],
      pendingNodeIds: ['image-2'],
    })
    expect(harness.flushAutosave).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.observeTask).toHaveBeenCalledOnce()
    expect(vi.mocked(observation.resolve).mock.calls.map(([index]) => index)).toEqual([0, 1, 1])
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'succeeded',
    ])
    expect(harness.onMaterialized).toHaveBeenCalledTimes(2)
    expect(harness.flushAutosave).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('bounds repeated pending display retries with the existing backoff', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    const observation = {
      resolve: vi.fn<Observation['resolve']>(async () => {
        throw new DOMException('Still preparing', 'TimeoutError')
      }),
    }
    harness.observeTask.mockResolvedValue(observation)
    const recovery = harness.coordinator.recover()
    await flushPromises()
    await recovery
    await vi.advanceTimersByTimeAsync(675_000)
    expect(harness.observeTask).toHaveBeenCalledOnce()
    expect(observation.resolve).toHaveBeenCalledTimes(18)
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'running',
      'running',
    ])
    expect(harness.flushAutosave).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('reports a background autosave failure through the error boundary and cleans up', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    const failure = new Error('autosave failed')
    harness.flushAutosave.mockRejectedValue(failure)
    harness.observeTask
      .mockImplementationOnce(() => new Promise(() => undefined))
      .mockResolvedValue(completedObservation())
    const recovery = harness.coordinator.recover()
    await vi.advanceTimersByTimeAsync(360_000)
    await recovery
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.onError).toHaveBeenCalledExactlyOnceWith(failure)
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'succeeded',
    ])
    expect(vi.getTimerCount()).toBe(0)
    await harness.coordinator.recover()
    expect(harness.observeTask).toHaveBeenCalledTimes(2)
  })

  it.each(
    (['detach', 'supersede', 'delete', 'dispose', 'switch-board'] as const).flatMap((action) =>
      (['backoff', 'observation'] as const).map((phase) => ({ action, phase })),
    ),
  )('cleans up $phase and ignores late results after $action', async ({ action, phase }) => {
    vi.useFakeTimers()
    const harness = createHarness()
    let finishLate: ((value: Observation) => void) | undefined
    harness.observeTask
      .mockImplementationOnce(() => new Promise(() => undefined))
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishLate = resolve
          }),
      )
    const recovery = harness.coordinator.recover()
    await vi.advanceTimersByTimeAsync(360_000)
    await recovery
    if (phase === 'observation') await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.observeTask).toHaveBeenCalledTimes(phase === 'observation' ? 2 : 1)
    const signal = harness.observeTask.mock.calls[1]?.[1].signal
    const attachment = harness.core.currentAttachment()
    if (!attachment) throw new Error('Expected Board attachment')
    if (action === 'detach') harness.core.detachBoard()
    else if (action === 'dispose') harness.coordinator.dispose()
    else if (action === 'supersede') {
      harness.core.begin(attachment, 'image-1')
      if (signal) expect(signal.aborted).toBe(false)
      harness.core.begin(attachment, 'image-2')
    } else if (action === 'delete') {
      harness.core.release(harness.core.captureActiveLeases(['image-1', 'image-2']))
      harness.graph.store
        .beginTransaction('delete')
        .removeNode('image-1')
        .removeNode('image-2')
        .commit()
    } else {
      const graph = CanvasGraphSession.hydrate('board-b', createEmptyBoardSnapshot())
      harness.core.attachBoard(BoardGraphSession.create('board-b', { graphSession: graph }))
    }
    if (signal) expect(signal.aborted).toBe(true)
    const late = completedObservation()
    finishLate?.(late)
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(315_000)
    expect(late.resolve).not.toHaveBeenCalled()
    expect(harness.onMaterialized).not.toHaveBeenCalled()
    expect(harness.flushAutosave).not.toHaveBeenCalled()
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not release the claim of a newer exact lease when old background work ends', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    harness.observeTask.mockImplementation(() => new Promise(() => undefined))
    const recovery = harness.coordinator.recover()
    await vi.advanceTimersByTimeAsync(360_000)
    await recovery
    const attachment = harness.core.currentAttachment()
    if (!attachment) throw new Error('Expected Board attachment')
    const identity = {
      resultNodeId: 'image-1' as const,
      taskId: 'new-task',
      clientActionId: 'new-action',
    }
    const replacement = harness.core.hydratePendingLease(attachment, identity)
    if (replacement.kind !== 'started') throw new Error('Expected replacement lease')
    expect(harness.core.claimHydratedLease(attachment, identity)).toBe(replacement.lease)
    harness.core.begin(attachment, 'image-2')
    await flushPromises()
    expect(harness.core.claimHydratedLease(attachment, identity)).toBeNull()
    harness.coordinator.dispose()
  })
})
