import type { TaskImageOutcome, TaskImageOutcomeWaiter } from '../task-runner/taskImageOutcome'
import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '../../../core/graph'
import { BoardGraphSession } from '../pending-execution/boardGraphSession'
import { createPendingImageExecutionCore } from '../pending-execution/core'
import { collectPendingImageTasks } from '../pending-execution/taskSnapshot'
import { useCanvasExecutionWorkspace } from './useCanvasExecutionWorkspace'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'

beforeEach(() => {
  setMediaWorkspaceIdResolver(() => 'ws_test123')
})

afterEach(() => {
  resetMediaWorkspaceIdResolver()
})

vi.mock('./canvasTaskSubmissionAdapter', () => ({
  submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
}))

const WORKSPACE_ID = 'ws_test123'

function workspaceAssetEnvelope(assetId: string) {
  return {
    data: {
      id: assetId,
      workspace_id: WORKSPACE_ID,
      status: 'ready',
      media_kind: 'image',
      mime_type: 'image/webp',
      width: 400,
      height: 200,
      size_bytes: 1,
      checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${assetId}/content`,
      derivatives: {
        preview: {
          status: 'ready',
          url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${assetId}/derivatives/preview`,
          width: 400,
          height: 200,
        },
      },
      created_at: '2026-09-25T00:00:00.000Z',
      updated_at: '2026-09-25T00:00:00.000Z',
    },
  }
}

const mediaAssetMetadataHandler = http.get(
  '/api/v1/workspaces/:workspaceId/media-assets/:assetId',
  ({ params }) => HttpResponse.json(workspaceAssetEnvelope(String(params.assetId))),
)

beforeEach(() => {
  useMswHandlers(mediaAssetMetadataHandler)
})

const rendererMetrics = { readNodeMetrics: () => null }

function createStore(taskId = 'task-1', outputCount = 1) {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = Array.from({ length: outputCount }, (_, resultIndex) => ({
    id: resultIndex + 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: taskId,
    pendingResultIndex: resultIndex,
    pendingClientActionId: `action-${taskId}`,
    outputStatus: 'running',
    outputImages: [],
  }))
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
  waitTaskImageOutcome: TaskImageOutcomeWaiter,
  shouldHydrate = true,
) {
  const store = graphSession.store
  const attachment = core.currentAttachment()
  if (!attachment) throw new Error('Expected an attached Board')
  if (shouldHydrate) {
    collectPendingImageTasks(store.exportSnapshot()).forEach((pending) => {
      core.hydratePendingLease(attachment, pending)
    })
  }
  return useCanvasExecutionWorkspace({
    // Reattach facts must come from the Canvas graph snapshot, never this Flow projection.
    nodes: ref([
      { id: 'image-99', position: { x: 0, y: 0 }, data: { pendingTaskId: 'ui-only-task' } },
    ] as never),
    edges: ref([]),
    readCanvasGraphStore: () => store,
    rendererMetrics,
    boardBaseSnapshot: ref(store.exportSnapshot()),
    pendingResults: {
      reservePendingResults: vi.fn(),
      attachPendingTaskIds: vi.fn(),
      removePendingResults: vi.fn(),
    },
    executionCore: core,
    cancelTasks: vi.fn(),
    onSuccess: vi.fn(),
    onWarning: vi.fn(),
    waitTaskImageOutcome,
    loadImageSize: async () => ({ width: 400, height: 200 }),
  })
}

describe('useCanvasExecutionWorkspace', () => {
  it.each([1, 4])(
    'observes one Task for %i prepared outputs and flushes persisted task identity',
    async (outputCount) => {
      const store = createStore('task-1', outputCount)
      const core = createPendingImageExecutionCore()
      core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
      const flushAutosave = vi.fn(async () => undefined)
      const attachPendingTaskIds = vi.fn()
      const waitTaskImageOutcome = vi.fn<TaskImageOutcomeWaiter>(async () => ({
        kind: 'completed',
        results: Array.from({ length: outputCount }, (_, resultIndex) => ({
          assetId: `asset_result${resultIndex}`,
          workspaceId: WORKSPACE_ID,
          width: 400,
          height: 200,
          resultIndex,
        })),
      }))
      const workspace = useCanvasExecutionWorkspace({
        nodes: ref([]),
        edges: ref([]),
        readCanvasGraphStore: () => store.store,
        rendererMetrics,
        boardBaseSnapshot: ref(createEmptyBoardSnapshot()),
        pendingResults: {
          reservePendingResults: () => ({
            resultNodeIds: Array.from({ length: outputCount }, (_, index) => `image-${index + 1}`),
          }),
          attachPendingTaskIds,
          removePendingResults: vi.fn(),
        },
        executionCore: core,
        cancelTasks: vi.fn(),
        onSuccess: vi.fn(),
        onWarning: vi.fn(),
        flushAutosave,
        waitTaskImageOutcome,
        loadImageSize: async () => ({ width: 400, height: 200 }),
      })
      await workspace.commands.runGeneration({
        kind: 'generation',
        sourceNodeId: 'prompt-1',
        source: { url: '', ratio: 1 },
        referenceImages: [],
        prompt: 'draw',
        outputCount,
        aspectRatio: 'auto',
        resultOptions: {},
      } as never)
      expect(attachPendingTaskIds).toHaveBeenCalledWith(
        Array.from({ length: outputCount }, (_, resultIndex) =>
          expect.objectContaining({
            resultNodeId: `image-${resultIndex + 1}`,
            resultIndex,
            taskId: 'task-1',
            clientActionId: expect.any(String),
          }),
        ),
      )
      expect(waitTaskImageOutcome).toHaveBeenCalledOnce()
      expect(flushAutosave).toHaveBeenCalledWith({ isSilent: true })
      expect(core.snapshot().executions).toHaveLength(outputCount)
      expect(core.snapshot().executions.every((fact) => fact.status === 'succeeded')).toBe(true)
    },
  )

  it('reattaches a persisted result through Core without a second state owner', async () => {
    const store = createStore()
    store.store
      .beginTransaction('node-update')
      .updateNode('image-1', {
        pendingClientActionId: 'canvas:restored-action',
      })
      .commit()
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
    const workspace = createReattachWorkspace(store, core, async () => ({
      kind: 'completed' as const,
      results: [
        {
          assetId: 'asset_recovered',
          workspaceId: WORKSPACE_ID,
          resultIndex: 0,
        },
      ],
    }))
    await workspace.commands.reattachPendingImageResults()
    expect(core.factFor('image-1')).toMatchObject({
      status: 'succeeded',
      taskId: 'task-1',
      clientActionId: 'canvas:restored-action',
    })
  })

  it('recovers the available result of a cancelled four-output task and cancels missing indices', async () => {
    const snapshot = createStore().exportSnapshot()
    const first = snapshot.imageNodes[0]
    if (!first) throw new Error('Expected a pending image')
    snapshot.imageNodes = Array.from({ length: 4 }, (_, resultIndex) => ({
      ...first,
      id: resultIndex + 1,
      pendingResultIndex: resultIndex,
    }))
    const graph = CanvasGraphSession.hydrate('board-a', snapshot)
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: graph }))
    const waitTaskImageOutcome = vi.fn<TaskImageOutcomeWaiter>(async () => ({
      kind: 'cancelled',
      results: [{ workspaceId: WORKSPACE_ID, assetId: 'asset_partial', resultIndex: 0 }],
    }))
    const workspace = createReattachWorkspace(graph, core, waitTaskImageOutcome)
    await workspace.commands.reattachPendingImageResults()
    expect(core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'cancelled',
      'cancelled',
      'cancelled',
    ])
    expect(graph.exportSnapshot().imageNodes[0]?.sourceImageUrl).not.toBe('')
    expect(waitTaskImageOutcome).toHaveBeenCalledOnce()
  })

  it('hydrates a pending lease as part of the recovery flow', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
    const waitTaskImageOutcome = vi.fn(async () => ({
      kind: 'completed' as const,
      results: [
        {
          assetId: 'asset_recovered',
          workspaceId: WORKSPACE_ID,
          resultIndex: 0,
        },
      ],
    }))
    const workspace = createReattachWorkspace(store, core, waitTaskImageOutcome, false)

    await workspace.commands.reattachPendingImageResults()

    expect(waitTaskImageOutcome).toHaveBeenCalledOnce()
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('claims one hydration lease only once across concurrent reattach drivers', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<TaskImageOutcome>()
    const waitTaskImageOutcome = vi.fn(() => deferred.promise)
    const workspace = createReattachWorkspace(store, core, waitTaskImageOutcome)

    const first = workspace.commands.reattachPendingImageResults()
    const duplicate = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(waitTaskImageOutcome).toHaveBeenCalledOnce())
    deferred.resolve({
      kind: 'completed',
      results: [
        {
          assetId: 'asset_recovered',
          workspaceId: WORKSPACE_ID,
          resultIndex: 0,
        },
      ],
    })
    await Promise.all([first, duplicate])

    expect(waitTaskImageOutcome).toHaveBeenCalledOnce()
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('ignores a stale reattach poll after the same slot receives a newer exact lease', async () => {
    const store = createStore('task-old')
    const core = createPendingImageExecutionCore()
    const attachment = core.attachBoard(
      BoardGraphSession.create('board-a', { graphSession: store }),
    )
    const deferred = createDeferred<TaskImageOutcome>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() =>
      expect(core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-old' }),
    )
    const superseding = core.begin(attachment, 'image-1')
    if (superseding.kind !== 'started') throw new Error('Expected superseding execution')
    core.markSubmitted(superseding.lease, 'task-new', 'action-task-new')
    deferred.resolve({
      kind: 'completed',
      results: [{ url: 'https://example.com/old.png', resultIndex: 0 }],
    })
    await reattach

    expect(core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-new' })
    expect(store.exportSnapshot().imageNodes[0]).toMatchObject({
      pendingTaskId: 'task-old',
      outputStatus: 'running',
    })
  })

  it('does not let an old-board poll materialize after a board switch', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<TaskImageOutcome>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(core.factFor('image-1')).toMatchObject({ status: 'running' }))
    const boardBStore = CanvasGraphSession.hydrate('board-b', createEmptyBoardSnapshot())
    core.attachBoard(BoardGraphSession.create('board-b', { graphSession: boardBStore }))
    deferred.resolve({
      kind: 'completed',
      results: [
        {
          assetId: 'asset_old_board',
          workspaceId: WORKSPACE_ID,
          resultIndex: 0,
        },
      ],
    })
    await reattach

    expect(core.snapshot().attachment).toMatchObject({ boardId: 'board-b' })
    expect(core.factFor('image-1')).toBeNull()
    expect(store.exportSnapshot().imageNodes[0]).toMatchObject({
      pendingTaskId: 'task-1',
      outputStatus: 'running',
    })
  })

  it('turns a reattach poll for a deleted domain node into a rejected materialization', async () => {
    const store = createStore()
    const core = createPendingImageExecutionCore()
    core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
    const deferred = createDeferred<TaskImageOutcome>()
    const workspace = createReattachWorkspace(store, core, () => deferred.promise)

    const reattach = workspace.commands.reattachPendingImageResults()
    await vi.waitFor(() => expect(core.factFor('image-1')).toMatchObject({ status: 'running' }))
    store.store.beginTransaction('delete').removeNode('image-1').commit()
    deferred.resolve({
      kind: 'completed',
      results: [
        {
          assetId: 'asset_deleted',
          workspaceId: WORKSPACE_ID,
          resultIndex: 0,
        },
      ],
    })
    await reattach

    expect(core.factFor('image-1')).toMatchObject({ status: 'cancelled', taskId: 'task-1' })
    expect(store.exportSnapshot().imageNodes).toEqual([])
  })
})
