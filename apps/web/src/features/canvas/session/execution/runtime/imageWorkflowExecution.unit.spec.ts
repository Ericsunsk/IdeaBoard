import { effectScope, ref, shallowRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TaskCancelledError } from '../task-runner/taskResultErrors'
import { createEmptyBoardSnapshot } from '../../../../board'
import { setStoredAuth } from '../../../../../core/utils/auth'
import { getWorkspaceSelectionStore } from '../../../../workspace'

beforeEach(() => {
  setStoredAuth({
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'tester',
    avatar: '',
    role: 'user',
  })
  getWorkspaceSelectionStore().select('ws_test123')
})
import { useCanvasVueFlowGraphAdapter } from '../../../session/flow-adapter'
import {
  createFlowGraphCommandOptions,
  readFlowGraphCommandSideEffects,
} from '../../../session/flow-adapter'
import { CanvasGraphSession } from '../../board-session'
import { bindCanvasGraphSessionProjection } from '../../../session/flow-adapter'
import { boardSnapshotToFlow } from '../../../session/flow-adapter'
import { BoardGraphSession } from '../pending-execution/boardGraphSession'
import { createPendingImageExecutionCore } from '../pending-execution/core'
import { useImageWorkflowExecution } from './imageWorkflowExecution'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'
import { assemblePendingImageWorkflow } from './pendingWorkflowAssembly'
import { getMediaAssetMetadata } from '@/api/media/mediaAssetDelivery'

vi.mock('../../../../../core/media/imageDimensions', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../../core/media/imageDimensions')>()),
  readImageNaturalSize: vi.fn(async () => ({ width: 1200, height: 800 })),
}))
vi.mock('../media/inpaintMask', () => ({
  prepareInpaintMaskAsset: vi.fn(async () => 'asset_mask1'),
}))
vi.mock('../workspace/canvasWorkspaceTaskAdapter', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../workspace/canvasWorkspaceTaskAdapter')>()
  return {
    ...actual,
    requireCanvasWorkspaceId: vi.fn(() => 'ws_test123'),
  }
})
vi.mock('@/api/media/mediaAssetDelivery', () => ({
  getMediaAssetMetadata: vi.fn(async () => ({ width: 466, height: 516 })),
  resolveMediaAssetProcessingURL: vi.fn(
    async (assetId: string) => `/api/v1/workspaces/ws_test123/media-assets/${assetId}/content`,
  ),
}))

const rendererMetrics = { readNodeMetrics: () => null }

function imageActionPlan() {
  return {
    kind: 'image-action',
    activationId: 'a',
    sourceNodeId: 'image-1',
    source: { url: 'https://example.com/a.png', ratio: 1, assetId: 'asset_source42' },
    action: 'cutout',
    prompt: '',
    outputCount: 1,
    resultOptions: {},
  } as const
}

function createPublicHarness(transport: PendingImageWorkflowTaskTransport) {
  const base = createEmptyBoardSnapshot()
  base.imageNodes.push({
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/a.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
  })
  const flow = boardSnapshotToFlow(base)
  const nodes = shallowRef<Node[]>(flow.nodes)
  const edges = shallowRef<Edge[]>(flow.edges)
  const markDirty = vi.fn()
  const flushAutosave = vi.fn(async (_options?: { isSilent?: boolean }) => undefined)
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    afterCommand: (result) => {
      const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
      if (result.didChange && sideEffects?.shouldMarkDirty !== false) {
        markDirty()
        void flushAutosave({ isSilent: true })
      }
    },
  })
  const core = createPendingImageExecutionCore()
  const graphSession = CanvasGraphSession.hydrate('board-a', base)
  bindCanvasGraphSessionProjection({
    session: graphSession,
    graphAdapter,
    readCurrent: () => ({ nodes: nodes.value, edges: edges.value }),
    projectFull: () => boardSnapshotToFlow(graphSession.exportSnapshot(base)),
    isCurrentAttachment: () => true,
  })
  core.attachBoard(
    BoardGraphSession.create('board-a', {
      graphSession,
      onMaterialized: (store) => {
        const next = boardSnapshotToFlow(store.exportSnapshot(base))
        graphAdapter.replaceGraph(
          { nodes: next.nodes, edges: next.edges },
          createFlowGraphCommandOptions({ shouldRecordHistory: false }),
        )
      },
    }),
  )
  const execution = useImageWorkflowExecution({
    nodes,
    edges,
    readCanvasGraphStore: () => graphSession.store,
    rendererMetrics,
    boardBaseSnapshot: ref(base),
    executionWorkspace: {
      state: {
        executionCore: core,
        isNodeRunning: (id) => core.captureActiveLeases([id]).length > 0,
      },
      commands: {
        runGeneration: vi.fn(),
        hydratePendingImageLeases: vi.fn(),
        reattachPendingImageResults: vi.fn(),
      },
    },
    cancelTasks: vi.fn(),
    onSuccess: vi.fn(),
    onWarning: vi.fn(),
    taskTransport: transport,
  })
  return { core, execution, flushAutosave, markDirty, nodes }
}

describe('useImageWorkflowExecution', () => {
  it('uses original asset metadata for variations before submitting an auto-sized task', async () => {
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
      taskId: 'task-auto',
    }))
    const harness = createPublicHarness({
      submit,
      observe: async () => ({
        resolve: async () => ({ kind: 'completed', result: { url: 'result', ratio: 1 } }),
      }),
    })
    const plan = { ...imageActionPlan(), action: 'variations', outputResolution: '2k' }

    await harness.execution.executeImageWorkflow(plan as never)

    expect(getMediaAssetMetadata).toHaveBeenCalledWith('asset_source42')
    expect(submit.mock.calls[0]?.[0].params).toMatchObject({
      ratio: 'auto',
      original_image_width: 466,
      original_image_height: 516,
      output_resolution: '2k',
    })
    expect(submit.mock.calls[0]?.[0].params).not.toHaveProperty('custom_width')
  })

  it('does not submit variations when original asset metadata cannot be read', async () => {
    vi.mocked(getMediaAssetMetadata).mockRejectedValueOnce(new Error('Metadata unavailable'))
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>()
    const harness = createPublicHarness({ submit, observe: vi.fn() })

    await expect(
      harness.execution.executeImageWorkflow({
        ...imageActionPlan(),
        action: 'variations',
      } as never),
    ).rejects.toThrow('Metadata unavailable')
    expect(submit).not.toHaveBeenCalled()
  })

  it('does not show a stale save warning when the scope is disposed during a pending flush', async () => {
    const scope = effectScope()
    const graphSession = CanvasGraphSession.hydrate('board-a', createEmptyBoardSnapshot())
    let rejectFlush!: (error: unknown) => void
    const onWarning = vi.fn()
    const assembly = scope.run(() =>
      assemblePendingImageWorkflow({
        nodes: ref([]),
        edges: ref([]),
        readCanvasGraphStore: () => graphSession.store,
        rendererMetrics,
        executionCore: createPendingImageExecutionCore(),
        isNodeExecutionActive: () => false,
        cancelTasks: vi.fn(),
        onSuccess: vi.fn(),
        onWarning,
        flushAutosave: () =>
          new Promise((_, reject) => {
            rejectFlush = reject
          }),
      }),
    )
    if (!assembly) throw new Error('Expected active assembly scope')
    const log = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const flushing = assembly.flushPendingTaskSnapshot()
    scope.stop()
    rejectFlush(new Error('save response lost'))
    await flushing
    expect(onWarning).not.toHaveBeenCalled()
    log.mockRestore()
  })

  it('materializes through Graph Session projection and marks the board dirty for autosave', async () => {
    const harness = createPublicHarness({
      submit: async () => ({ taskId: 'task-1' }),
      observe: async () => ({
        resolve: async () => ({
          kind: 'completed',
          result: { url: 'https://example.com/result.png', ratio: 1 },
        }),
      }),
    })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(
      harness.nodes.value.find((node) => node.id !== 'image-1')?.data.snapshotNode,
    ).toMatchObject({ sourceImageUrl: 'https://example.com/result.png', outputStatus: 'ready' })
    expect(harness.markDirty).toHaveBeenCalled()
    expect(harness.flushAutosave).toHaveBeenCalledWith({ isSilent: true })
  })

  it('cancels a real reserved pending node when submit is cancelled', async () => {
    const harness = createPublicHarness({
      submit: async () => {
        throw new TaskCancelledError()
      },
      observe: async () => ({
        resolve: async () => ({ kind: 'completed', result: { url: '', ratio: 1 } }),
      }),
    })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1'])
  })

  it('does not remove a superseding pending node when an old submit resolves late', async () => {
    let resolveSubmit!: (value: { taskId: string }) => void
    const harness = createPublicHarness({
      submit: async () =>
        await new Promise((resolve) => {
          resolveSubmit = resolve
        }),
      observe: async () => ({
        resolve: async () => ({ kind: 'completed', result: { url: '', ratio: 1 } }),
      }),
    })
    const running = harness.execution.executeImageWorkflow(imageActionPlan())
    await vi.waitFor(() => expect(harness.nodes.value).toHaveLength(2))
    const resultNodeId = harness.nodes.value.find((node) => node.id !== 'image-1')!
      .id as `image-${number}`
    const attachment = harness.core.currentAttachment()!
    const superseding = harness.core.begin(attachment, resultNodeId)
    if (superseding.kind !== 'started') throw new Error('Expected superseding execution')
    harness.core.markSubmitted(superseding.lease, 'task-new', 'action-task-new')
    resolveSubmit({ taskId: 'task-old' })
    await running
    expect(harness.nodes.value.some((node) => node.id === resultNodeId)).toBe(true)
    expect(harness.core.factFor(resultNodeId)).toMatchObject({
      status: 'running',
      taskId: 'task-new',
    })
  })

  it('passes a prepared image action to the real Runner transport without per-run callbacks', async () => {
    const submit = vi.fn(async () => {
      throw new Error('submit failed')
    })
    const harness = createPublicHarness({
      submit,
      observe: vi.fn(),
    })
    await harness.execution.executeImageWorkflow({
      kind: 'image-action',
      activationId: 'a',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1, assetId: 'asset_source42' },
      action: 'cutout',
      prompt: '',
      outputCount: 1,
      resultOptions: {},
    } as never)
    expect(submit).toHaveBeenCalledOnce()
    expect(harness.core.snapshot().executions).toEqual([])
  })

  it('uses the complete outpaint request for submission', async () => {
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
      taskId: 'task-extend',
    }))
    const harness = createPublicHarness({
      submit,
      observe: async () => ({
        resolve: async () => ({
          kind: 'completed',
          result: { url: 'https://example.com/extended.png', ratio: 0.75 },
        }),
      }),
    })

    await harness.execution.executeImageWorkflow({
      kind: 'outpaint',
      activationId: 'extend',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1, assetId: 'asset_source42' },
      prompt: 'extend the horizon',
      outputCount: 1,
      frameRect: { left: -40, top: -10, width: 160, height: 140 },
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      targetRatio: 0.75,
      targetWidth: 320,
      resultOptions: {},
    } as never)

    const submittedRequest = submit.mock.calls[0]?.[0]
    expect(submittedRequest).toMatchObject({
      service: 'outpaint',
      media_asset_ids: ['asset_source42'],
      params: {
        output_count: 1,
        original_image_width: 1200,
        original_image_height: 800,
        outpaint: {
          target_width: 1920,
          target_height: 1120,
          image_offset_x: 480,
          image_offset_y: 80,
        },
      },
    })
  })

  it('uses the complete sized inpaint request for submission', async () => {
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
      taskId: 'task-inpaint',
    }))
    const harness = createPublicHarness({
      submit,
      observe: async () => ({
        resolve: async () => ({
          kind: 'completed',
          result: { url: 'https://example.com/inpainted.png', ratio: 1 },
        }),
      }),
    })

    await harness.execution.executeImageWorkflow({
      kind: 'inpaint',
      activationId: 'inpaint',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1, assetId: 'asset_source42' },
      prompt: 'remove the object',
      rawMaskDataUrl: 'data:image/png;base64,mask',
      outputCount: 1,
      resultOptions: {},
    } as never)

    const submittedRequest = submit.mock.calls[0]?.[0]
    expect(submittedRequest).toMatchObject({
      service: 'inpaint',
      prompt: 'remove the object',
      media_asset_ids: ['asset_source42'],
      params: {
        output_count: 1,
        mask_asset_id: 'asset_mask1',
        original_image_width: 1200,
        original_image_height: 800,
        custom_width: 1200,
        custom_height: 800,
      },
    })
    expect(submittedRequest?.params).not.toHaveProperty('mask_image_url')
    expect(submittedRequest?.params).not.toHaveProperty('reference_image_urls')
  })

  it('delegates prompt execution and pending reattach to the shared Core workspace seam', async () => {
    const core = createPendingImageExecutionCore()
    const graphSession = CanvasGraphSession.hydrate('board-a', createEmptyBoardSnapshot())
    core.attachBoard(
      BoardGraphSession.create('board-a', {
        graphSession,
      }),
    )
    const workspace = {
      state: { executionCore: core, isNodeRunning: () => false },
      commands: {
        runGeneration: vi.fn(),
        hydratePendingImageLeases: vi.fn(),
        reattachPendingImageResults: vi.fn(),
      },
    }
    const execution = useImageWorkflowExecution({
      nodes: ref([]),
      edges: ref([]),
      readCanvasGraphStore: () => graphSession.store,
      rendererMetrics,
      boardBaseSnapshot: ref(createEmptyBoardSnapshot()),
      executionWorkspace: workspace,
      cancelTasks: vi.fn(),
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
      taskTransport: { submit: vi.fn(), observe: vi.fn() },
    })
    const plan = {
      kind: 'generation',
      activationId: 'prompt',
      sourceNodeId: 'prompt-1',
      source: { url: '', ratio: 1 },
      referenceImages: [],
      prompt: 'draw',
      outputCount: 1,
      aspectRatio: 'auto',
      resultOptions: {},
    } as never
    await execution.executeImageWorkflow(plan)
    await execution.reattachPendingImageResults()
    expect(workspace.commands.runGeneration).toHaveBeenCalledWith(plan)
    expect(workspace.commands.reattachPendingImageResults).toHaveBeenCalledOnce()
  })
})
