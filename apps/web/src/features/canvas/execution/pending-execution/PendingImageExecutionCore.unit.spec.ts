import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type ImageNodeItem,
} from '../../../canvas-board/domain'
import { CanvasGraphStore } from '../../graph-runtime'
import {
  CanvasBoardGraphSession,
  createPendingImageExecutionCore,
  PendingImageMaterializationError,
  PendingImageMutationContractError,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
  type PendingImageExecutionStartResult,
  type PendingImageResultNodeId,
} from './index'
import type { CanvasBoardAttachment } from '../../board-attachment'
import {
  bindCanvasBoardGraphSession,
  type PendingImageDomainMutationResult,
} from './CanvasBoardGraphSession'

function imageNodeId(id: number): PendingImageResultNodeId {
  return `image-${id}`
}

function createPendingImageNode(id: number, taskId: string): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: id * 40,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: taskId,
    pendingResultIndex: id - 1,
    pendingClientActionId: `action-${taskId}`,
    outputStatus: 'running',
    outputImages: [],
  }
}

function createPendingResultStore(taskIds: readonly string[]): CanvasGraphStore {
  const snapshot: CanvasBoardSnapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes = taskIds.map((taskId, index) => createPendingImageNode(index + 1, taskId))
  return CanvasGraphStore.fromSnapshot(snapshot)
}

function requireLease(result: PendingImageExecutionStartResult): PendingImageExecutionLease {
  if (result.kind !== 'started') throw new Error(`Expected started, received ${result.reason}`)
  return result.lease
}

function attachCore(
  store: CanvasGraphStore,
  boardId = 'board-a',
): {
  core: PendingImageExecutionCore
  attachment: CanvasBoardAttachment
} {
  const core = createPendingImageExecutionCore()
  const attachment = core.attachBoard(
    CanvasBoardGraphSession.create(boardId, { createGraphStore: () => store }),
  )
  return { core, attachment }
}

function hydrateLease(
  core: PendingImageExecutionCore,
  attachment: CanvasBoardAttachment,
  resultNodeId: PendingImageResultNodeId,
  taskId: string,
  clientActionId = `action-${taskId}`,
): PendingImageExecutionStartResult {
  return core.hydratePendingLease(attachment, { resultNodeId, taskId, clientActionId })
}

const mutationContractViolations: Array<{
  name: string
  result: PendingImageDomainMutationResult
}> = [
  {
    name: 'missing candidate outcome',
    result: { didChange: false, changedNodeIds: [], outcomes: [] },
  },
  {
    name: 'duplicate candidate outcome',
    result: {
      didChange: false,
      changedNodeIds: [],
      outcomes: [
        { resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'missing-node' },
        { resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'missing-node' },
      ],
    },
  },
  {
    name: 'extra outcome',
    result: {
      didChange: false,
      changedNodeIds: [],
      outcomes: [
        { resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'missing-node' },
        { resultNodeId: imageNodeId(99), kind: 'rejected', reason: 'missing-node' },
      ],
    },
  },
  {
    name: 'changed nodes disagree with materialized outcomes',
    result: {
      didChange: false,
      changedNodeIds: [],
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'materialized' }],
    },
  },
]

describe('PendingImageExecutionCore', () => {
  it('materializes current slots in one transaction and rejects partial stale slots', () => {
    const store = createPendingResultStore(['task-1', 'task-2', 'task-current'])
    const beginTransaction = vi.spyOn(store, 'beginTransaction')
    const { core, attachment } = attachCore(store)
    const firstLease = requireLease(core.begin(attachment, imageNodeId(1)))
    expect(core.markSubmitted(firstLease, 'task-1', 'action-task-1')).toMatchObject({
      kind: 'applied',
      fact: { status: 'running', taskId: 'task-1' },
    })
    const secondLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(2), 'task-2'),
    )
    const staleLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(3), 'task-stale'),
    )
    const currentLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(3), 'task-current'),
    )

    const completion = core.materializeBatch([
      {
        lease: firstLease,
        result: { url: 'https://example.com/result-1.png', ratio: 1 },
      },
      {
        lease: staleLease,
        result: { url: 'https://example.com/stale.png', ratio: 1 },
      },
      {
        lease: secondLease,
        result: { url: 'https://example.com/result-2.png', ratio: 0.5 },
      },
    ])

    expect(beginTransaction).toHaveBeenCalledTimes(1)
    expect(completion).toMatchObject({
      didChange: true,
      changedNodeIds: [imageNodeId(1), imageNodeId(2)],
      outcomes: [
        { resultNodeId: imageNodeId(1), kind: 'materialized' },
        { resultNodeId: imageNodeId(3), kind: 'rejected', reason: 'stale-execution' },
        { resultNodeId: imageNodeId(2), kind: 'materialized' },
      ],
    })
    expect(core.factFor(imageNodeId(1))).toMatchObject({ status: 'succeeded' })
    expect(core.factFor(imageNodeId(2))).toMatchObject({ status: 'succeeded' })
    expect(core.factFor(imageNodeId(3))).toEqual({
      resultNodeId: imageNodeId(3),
      status: 'running',
      taskId: 'task-current',
      clientActionId: 'action-task-current',
    })
    expect(core.isCurrent(currentLease, 'task-current')).toBe(true)
  })

  it('publishes domain-neutral immutable snapshots and supports unsubscribe', () => {
    const store = createPendingResultStore(['task-1'])
    const core = createPendingImageExecutionCore()
    const snapshots = [core.snapshot()]
    const unsubscribe = core.subscribe((snapshot) => snapshots.push(snapshot))
    const attachment = core.attachBoard(
      CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store }),
    )
    const lease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-1'),
    )
    core.fail(lease)

    expect(snapshots).toHaveLength(4)
    expect(snapshots.map((snapshot) => snapshot.version)).toEqual([0, 1, 2, 3])
    expect(snapshots[2]).toEqual({
      version: 2,
      attachment: { boardId: 'board-a', epoch: attachment.epoch },
      executions: [
        {
          resultNodeId: imageNodeId(1),
          status: 'running',
          taskId: 'task-1',
          clientActionId: 'action-task-1',
        },
      ],
    })
    expect(Object.isFrozen(snapshots[2])).toBe(true)
    expect(Object.isFrozen(snapshots[2]?.attachment)).toBe(true)
    expect(Object.isFrozen(snapshots[2]?.executions)).toBe(true)
    expect(Object.isFrozen(snapshots[2]?.executions[0])).toBe(true)

    unsubscribe()
    core.detachBoard()
    expect(snapshots).toHaveLength(4)
  })

  it('notifies every subscriber and reports a listener failure without interrupting the command', () => {
    const store = createPendingResultStore(['task-1'])
    const subscriberError = new Error('subscriber failed')
    const onSubscriberError = vi.fn()
    const laterSubscriber = vi.fn()
    const core = createPendingImageExecutionCore({ onSubscriberError })
    core.subscribe(() => {
      throw subscriberError
    })
    core.subscribe(laterSubscriber)

    expect(() => {
      core.attachBoard(CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store }))
    }).not.toThrow()

    expect(onSubscriberError).toHaveBeenCalledOnce()
    expect(onSubscriberError).toHaveBeenCalledWith(subscriberError)
    expect(laterSubscriber).toHaveBeenCalledOnce()
    expect(core.snapshot().attachment).toMatchObject({ boardId: 'board-a' })
    expect(core.snapshot().attachment?.epoch).toBeGreaterThan(0)
  })

  it('rejects old epochs and different Graph Sessions even when boardId is unchanged', () => {
    const firstStore = createPendingResultStore(['task-first'])
    const nextStore = createPendingResultStore(['task-next'])
    const core = createPendingImageExecutionCore()
    const firstSession = CanvasBoardGraphSession.create('board-a', {
      createGraphStore: () => firstStore,
    })
    const firstAttachment = core.attachBoard(firstSession)
    const oldEpochLease = requireLease(
      hydrateLease(core, firstAttachment, imageNodeId(1), 'task-first'),
    )

    const nextEpochAttachment = core.attachBoard(firstSession)
    expect(nextEpochAttachment.epoch).toBeGreaterThan(firstAttachment.epoch)
    expect(
      core.materializeBatch([
        {
          lease: oldEpochLease,
          result: { url: 'https://example.com/old-epoch.png', ratio: 1 },
        },
      ]),
    ).toMatchObject({
      didChange: false,
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'stale-execution' }],
    })
    expect(firstStore.exportSnapshot().imageNodes[0]).toMatchObject({ sourceImageUrl: '' })

    const previousSessionLease = requireLease(
      hydrateLease(core, nextEpochAttachment, imageNodeId(1), 'task-first'),
    )
    const nextSession = CanvasBoardGraphSession.create('board-a', {
      createGraphStore: () => nextStore,
    })
    core.attachBoard(nextSession)

    expect(
      core.materializeBatch([
        {
          lease: previousSessionLease,
          result: { url: 'https://example.com/old-session.png', ratio: 1 },
        },
      ]),
    ).toMatchObject({
      didChange: false,
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'stale-execution' }],
    })
    expect(firstStore.exportSnapshot().imageNodes[0]).toMatchObject({ sourceImageUrl: '' })
    expect(nextStore.exportSnapshot().imageNodes[0]).toMatchObject({
      sourceImageUrl: '',
      pendingTaskId: 'task-next',
    })
  })

  it('rejects a lease from another Board without writing the current Graph Session', () => {
    const boardAStore = createPendingResultStore(['task-a'])
    const boardBStore = createPendingResultStore(['task-b'])
    const core = createPendingImageExecutionCore()
    const boardAAttachment = core.attachBoard(
      CanvasBoardGraphSession.create('board-a', { createGraphStore: () => boardAStore }),
    )
    const boardALease = requireLease(
      hydrateLease(core, boardAAttachment, imageNodeId(1), 'task-a'),
    )
    core.attachBoard(
      CanvasBoardGraphSession.create('board-b', { createGraphStore: () => boardBStore }),
    )

    expect(
      core.materializeBatch([
        {
          lease: boardALease,
          result: { url: 'https://example.com/wrong-board.png', ratio: 1 },
        },
      ]),
    ).toMatchObject({
      didChange: false,
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'rejected', reason: 'wrong-board' }],
    })
    expect(boardBStore.exportSnapshot().imageNodes[0]).toMatchObject({
      sourceImageUrl: '',
      pendingTaskId: 'task-b',
    })
  })

  it('returns a shared Task only after its final active slot is cancelled or released', () => {
    const store = createPendingResultStore(['task-shared', 'task-shared'])
    const { core, attachment } = attachCore(store)
    const firstLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-shared', 'action-shared-1'),
    )
    const secondLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(2), 'task-shared', 'action-shared-2'),
    )

    expect(core.cancel([firstLease])).toEqual({
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'cancelled' }],
      cancellableTaskIds: [],
    })
    expect(core.release([secondLease])).toEqual({
      outcomes: [{ resultNodeId: imageNodeId(2), kind: 'released' }],
      cancellableTaskIds: ['task-shared'],
    })
    expect(core.factFor(imageNodeId(1))).toMatchObject({ status: 'cancelled' })
    expect(core.factFor(imageNodeId(2))).toBeNull()
  })

  it('restores the persisted client action identity with a domain pending fact', () => {
    const store = createPendingResultStore(['task-1'])
    const { core, attachment } = attachCore(store)
    const lease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-1', 'action-1'),
    )
    expect(core.factFor(imageNodeId(1))).toMatchObject({
      status: 'running', taskId: 'task-1', clientActionId: 'action-1',
    })
    expect(core.isCurrent(lease, 'task-1')).toBe(true)
    expect(core.claimHydratedLease(attachment, {
      resultNodeId: imageNodeId(1),
      taskId: 'task-1',
      clientActionId: 'action-1',
    })).toBe(lease)
    expect(core.claimHydratedLease(attachment, {
      resultNodeId: imageNodeId(1),
      taskId: 'task-1',
      clientActionId: 'action-1',
    })).toBeNull()
    expect(core.claimHydratedLease(attachment, {
      resultNodeId: imageNodeId(1),
      taskId: 'task-1',
      clientActionId: 'action-replaced',
    })).toBeNull()
  })

  it('requires a client action identity when hydration creates a lease', () => {
    const store = createPendingResultStore(['task-1'])
    const { core, attachment } = attachCore(store)

    expect(() => core.hydratePendingLease(attachment, {
      resultNodeId: imageNodeId(1),
      taskId: 'task-1',
      clientActionId: ' ',
    })).toThrow('clientActionId is required')
    expect(core.factFor(imageNodeId(1))).toBeNull()
  })

  it('reports superseded execution facts and protects a concurrent reattach with lease CAS', () => {
    const store = createPendingResultStore(['task-current'])
    const { core, attachment } = attachCore(store)
    const staleLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-stale'),
    )
    const next = hydrateLease(core, attachment, imageNodeId(1), 'task-current')
    const currentLease = requireLease(next)

    expect(next).toMatchObject({
      kind: 'started',
      superseded: {
        resultNodeId: imageNodeId(1),
        sequence: staleLease.sequence,
        status: 'running',
        taskId: 'task-stale',
      },
      cancellableTaskIds: ['task-stale'],
    })
    expect(core.fail(staleLease)).toEqual({ kind: 'rejected', reason: 'stale-execution' })
    expect(core.isCurrent(currentLease, 'task-current')).toBe(true)

    const sameTaskReattach = hydrateLease(core, attachment, imageNodeId(1), 'task-current')
    expect(sameTaskReattach).toMatchObject({
      kind: 'started',
      superseded: { taskId: 'task-current' },
      cancellableTaskIds: [],
    })
  })

  it('settles every current slot as failed when the batch mutation throws', () => {
    const store = createPendingResultStore(['task-1', 'task-2'])
    const core = createPendingImageExecutionCore()
    const mutationError = new Error('graph write failed')
    vi.spyOn(store, 'beginTransaction').mockImplementation(() => {
      throw mutationError
    })
    const attachment = core.attachBoard(
      CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store }),
    )
    const firstLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-1'),
    )
    const secondLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(2), 'task-2'),
    )

    let thrown: unknown
    try {
      core.materializeBatch([
        { lease: firstLease, result: { url: 'https://example.com/1.png', ratio: 1 } },
        { lease: secondLease, result: { url: 'https://example.com/2.png', ratio: 1 } },
      ])
    } catch (error) {
      thrown = error
    }

    expect(thrown).toBeInstanceOf(PendingImageMaterializationError)
    expect(thrown).toMatchObject({
      cause: mutationError,
      outcomes: [
        { resultNodeId: imageNodeId(1), kind: 'failed', reason: 'mutation-error' },
        { resultNodeId: imageNodeId(2), kind: 'failed', reason: 'mutation-error' },
      ],
    })
    expect(core.factFor(imageNodeId(1))).toMatchObject({ status: 'failed' })
    expect(core.factFor(imageNodeId(2))).toMatchObject({ status: 'failed' })
  })

  it.each(mutationContractViolations)(
    'fails closed when the mutation port returns $name',
    ({ result }) => {
      const store = createPendingResultStore(['task-1'])
      const session = CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store })
      const bindSession = session[bindCanvasBoardGraphSession]
      const bindSpy = vi
        .spyOn(CanvasBoardGraphSession.prototype, bindCanvasBoardGraphSession)
        .mockImplementationOnce(function (this: CanvasBoardGraphSession, epoch: number) {
          const binding = bindSession.call(this, epoch)
          return Object.freeze({
            ...binding,
            mutations: Object.freeze({
              ...binding.mutations,
              materializeBatch: () => result,
            }),
          })
        })
      const core = createPendingImageExecutionCore()
      const attachment = core.attachBoard(session)
      bindSpy.mockRestore()
      const lease = requireLease(
        hydrateLease(core, attachment, imageNodeId(1), 'task-1'),
      )

      let thrown: unknown
      try {
        core.materializeBatch([
          { lease, result: { url: 'https://example.com/result.png', ratio: 1 } },
        ])
      } catch (error) {
        thrown = error
      }

      expect(thrown).toBeInstanceOf(PendingImageMaterializationError)
      expect((thrown as PendingImageMaterializationError).cause).toBeInstanceOf(
        PendingImageMutationContractError,
      )
      expect(thrown).toMatchObject({
        outcomes: [{ resultNodeId: imageNodeId(1), kind: 'failed', reason: 'mutation-error' }],
      })
      expect(core.factFor(imageNodeId(1))).toMatchObject({ status: 'failed' })
    },
  )

  it('settles stale persisted task results per slot without blocking valid siblings', () => {
    const store = createPendingResultStore(['task-valid', 'task-snapshot'])
    const { core, attachment } = attachCore(store)
    const validLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(1), 'task-valid'),
    )
    const staleTaskLease = requireLease(
      hydrateLease(core, attachment, imageNodeId(2), 'task-runtime'),
    )

    const result = core.materializeBatch([
      { lease: validLease, result: { url: 'https://example.com/valid.png', ratio: 1 } },
      {
        lease: staleTaskLease,
        result: { url: 'https://example.com/stale-task.png', ratio: 1 },
      },
    ])

    expect(result).toMatchObject({
      didChange: true,
      changedNodeIds: [imageNodeId(1)],
      outcomes: [
        { resultNodeId: imageNodeId(1), kind: 'materialized' },
        { resultNodeId: imageNodeId(2), kind: 'rejected', reason: 'stale-task' },
      ],
    })
    expect(core.factFor(imageNodeId(1))).toMatchObject({ status: 'succeeded' })
    expect(core.factFor(imageNodeId(2))).toMatchObject({ status: 'cancelled' })
  })
})
