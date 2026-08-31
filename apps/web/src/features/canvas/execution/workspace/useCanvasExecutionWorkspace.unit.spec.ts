import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../../canvas-board/domain'
import { CanvasGraphSession } from '../../graph-runtime'
import {
  CanvasBoardGraphSession,
  collectPendingImageTasks,
  createPendingImageExecutionCore,
} from '../pending-execution'
import { useCanvasExecutionWorkspace } from './useCanvasExecutionWorkspace'

const rendererMetrics = { readNodeMetrics: () => null }

function createStore(taskId = 'task-1') {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes.push({ id: 1, type: 'image-node', x: 0, y: 0, width: 280, sourceImageUrl: '', sourceImageRatio: 1, pendingTaskId: taskId, pendingResultIndex: 0, pendingClientActionId: `action-${taskId}`, outputStatus: 'running', outputImages: [] })
  return CanvasGraphSession.hydrate('board-a', snapshot)
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve
  })
  return { promise, resolve }
}

function createReattachWorkspace(
  graphSession: CanvasGraphSession,
  core: ReturnType<typeof createPendingImageExecutionCore>,
  waitTaskImageResults: (taskId: string) => Promise<Array<{ url: string; resultIndex: number }>>,
  hydrate = true,
) {
  const store = graphSession.store
  const attachment = core.currentAttachment()
  if (!attachment) throw new Error('Expected an attached Board')
  if (hydrate) {
    collectPendingImageTasks(store.exportSnapshot()).forEach((pending) => {
      core.hydratePendingLease(attachment, pending)
    })
  }
  return useCanvasExecutionWorkspace({
    // Reattach facts must come from the Canvas graph snapshot, never this Flow projection.
    nodes: ref([{ id: 'image-99', position: { x: 0, y: 0 }, data: { pendingTaskId: 'ui-only-task' } }] as never),
    edges: ref([]), readCanvasGraphStore: () => store,
    rendererMetrics,
    boardBaseSnapshot: ref(store.exportSnapshot()),
    pendingResults: { reservePendingResults: vi.fn(), attachPendingTaskIds: vi.fn(), removePendingResults: vi.fn() },
    executionCore: core, cancelTasks: vi.fn(), onSuccess: vi.fn(), onWarning: vi.fn(),
    waitTaskImageResults, loadImageSize: async () => ({ width: 400, height: 200 }),
  })
}

describe('useCanvasExecutionWorkspace', () => {
  it('executes a prepared prompt intent through the real Runner and flushes persisted task identity', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const flushAutosave = vi.fn(async () => undefined)
    const attachPendingTaskIds = vi.fn()
    const workspace = useCanvasExecutionWorkspace({
      nodes: ref([]), edges: ref([]), readCanvasGraphStore: () => store.store,
      rendererMetrics,
      boardBaseSnapshot: ref(createEmptyCanvasBoardSnapshot()),
      pendingResults: { reservePendingResults: () => ({ resultNodeIds: ['image-1'] }), attachPendingTaskIds, removePendingResults: vi.fn() },
      executionCore: core, cancelTasks: vi.fn(), onSuccess: vi.fn(), onWarning: vi.fn(), flushAutosave,
      taskAdapter: { assertTaskRequestReady: vi.fn(async () => undefined), submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })) },
      waitTaskImageResults: async () => [{ url: 'https://example.com/result.png', width: 400, height: 200, resultIndex: 0 }], loadImageSize: async () => ({ width: 400, height: 200 }),
    })
    await workspace.commands.runGeneration({ kind: 'generation', sourceNodeId: 'prompt-1', source: { url: '', ratio: 1 }, referenceImages: [], prompt: 'draw', outputCount: 1, aspectRatio: 'auto', resultOptions: {} } as never)
    expect(attachPendingTaskIds).toHaveBeenCalledWith([expect.objectContaining({
      resultNodeId: 'image-1', resultIndex: 0, taskId: 'task-1', clientActionId: expect.any(String),
    })])
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded' })
  })

  it('reattaches a persisted result through Core without a second state owner', async () => {
    const store = createStore()
    store.store.beginTransaction('node-update').updateNode('image-1', {
      pendingClientActionId: 'canvas:restored-action',
    }).commit()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const workspace = createReattachWorkspace(
      store,
      core,
      async () => [{ url: 'https://example.com/recovered.png', resultIndex: 0 }],
    )
    await workspace.commands.reattachPendingImageResults()
    expect(core.factFor('image-1')).toMatchObject({
      status: 'succeeded', taskId: 'task-1', clientActionId: 'canvas:restored-action',
    })
  })

  it('hydrates a pending lease as part of the recovery flow', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const waitTaskImageResults = vi.fn(async () => [
      { url: 'https://example.com/recovered.png', resultIndex: 0 },
    ])
    const workspace = createReattachWorkspace(store, core, waitTaskImageResults, false)

    await workspace.commands.reattachPendingImageResults()

    expect(waitTaskImageResults).toHaveBeenCalledOnce()
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('claims one hydration lease only once across concurrent reattach drivers', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<Array<{ url: string; resultIndex: number }>>()
    const waitTaskImageResults = vi.fn(() => deferred.promise)
    const workspace = createReattachWorkspace(store, core, waitTaskImageResults)

    const first = workspace.commands.reattachPendingImageResults()
    const duplicate = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageResults).toHaveBeenCalledOnce())
    deferred.resolve([{ url: 'https://example.com/recovered.png', resultIndex: 0 }])
    await Promise.all([first, duplicate])

    expect(waitTaskImageResults).toHaveBeenCalledOnce()
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('ignores a stale reattach poll after the same slot receives a newer exact lease', async () => {
    const store = createStore('task-old')
    const core = createPendingImageExecutionCore()
    const attachment = core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<Array<{ url: string; resultIndex: number }>>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-old' }))
    const superseding = core.begin(attachment, 'image-1')
    if (superseding.kind !== 'started') throw new Error('Expected superseding execution')
    core.markSubmitted(superseding.lease, 'task-new', 'action-task-new')
    deferred.resolve([{ url: 'https://example.com/old.png', resultIndex: 0 }])
    await reattach

    expect(core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-new' })
    expect(store.exportSnapshot().imageNodes[0]).toMatchObject({ pendingTaskId: 'task-old', outputStatus: 'running' })
  })

  it('does not let an old-board poll materialize after a board switch', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<Array<{ url: string; resultIndex: number }>>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(core.factFor('image-1')).toMatchObject({ status: 'running' }))
    const boardBStore = CanvasGraphSession.hydrate('board-b', createEmptyCanvasBoardSnapshot())
    core.attachBoard(CanvasBoardGraphSession.create('board-b', { graphSession: boardBStore }))
    deferred.resolve([{ url: 'https://example.com/old-board.png', resultIndex: 0 }])
    await reattach

    expect(core.snapshot().attachment).toMatchObject({ boardId: 'board-b' })
    expect(core.factFor('image-1')).toBeNull()
    expect(store.exportSnapshot().imageNodes[0]).toMatchObject({ pendingTaskId: 'task-1', outputStatus: 'running' })
  })

  it('turns a reattach poll for a deleted domain node into a rejected materialization', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<Array<{ url: string; resultIndex: number }>>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(core.factFor('image-1')).toMatchObject({ status: 'running' }))
    store.store.beginTransaction('delete').removeNode('image-1').commit()
    deferred.resolve([{ url: 'https://example.com/deleted.png', resultIndex: 0 }])
    await reattach

    expect(core.factFor('image-1')).toMatchObject({ status: 'cancelled', taskId: 'task-1' })
    expect(store.exportSnapshot().imageNodes).toEqual([])
  })
})
