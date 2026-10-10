import { ref, shallowRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { TaskCancelledError } from '../task-runner/taskResultErrors'
import { createEmptyBoardSnapshot } from '../../../../board'
import { useCanvasVueFlowGraphAdapter } from '../../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import {
  createFlowGraphCommandOptions,
  readFlowGraphCommandSideEffects,
} from '../../../infrastructure/vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import { CanvasGraphSession } from '../../../domain/graph'
import { bindCanvasGraphSessionProjection } from '../../../infrastructure/vue-flow-adapter/bindCanvasGraphSessionProjection'
import { boardSnapshotToFlow } from '../../../infrastructure/vue-flow-adapter/flowSnapshotAdapter.ts'
import { BoardGraphSession, createPendingImageExecutionCore } from '../pending-execution'
import { useImageWorkflowExecution } from './imageWorkflowExecution'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'

vi.mock('../media/imageSize', () => ({
  loadImageSizeFromUrl: vi.fn(async () => ({ width: 1200, height: 800 })),
}))
vi.mock('../media/inpaintMask', () => ({
  prepareInpaintMask: vi.fn(async () => 'https://example.com/mask.png'),
  prepareInpaintMaskAsset: vi.fn(async () => 'asset_mask1'),
}))
vi.mock('../workspace/canvasWorkspaceTaskAdapter', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../workspace/canvasWorkspaceTaskAdapter')>()
  return {
    ...actual,
    requireCanvasWorkspaceId: vi.fn(() => 'ws_test123'),
  }
})
vi.mock('../../../../../api/mediaAssetDelivery', () => ({
  resolveMediaAssetProcessingURL: vi.fn(
    async (assetId: number) => `/api/v1/workspaces/ws_test123/media-assets/${assetId}/content`,
  ),
}))

const rendererMetrics = { readNodeMetrics: () => null }

function imageActionPlan() {
  return {
    kind: 'image-action',
    activationId: 'a',
    sourceNodeId: 'image-1',
    source: { url: 'https://example.com/a.png', ratio: 1 },
    action: 'cutout',
    prompt: '',
    outputCount: 1,
    resultOptions: {},
  } as never
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
  const flushAutosave = vi.fn(async (_options?: { silent?: boolean }) => undefined)
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    afterCommand: (result) => {
      const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
      if (result.didChange && sideEffects?.markDirty !== false) {
        markDirty()
        void flushAutosave({ silent: true })
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
          createFlowGraphCommandOptions({ recordHistory: false }),
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
  it('materializes through Graph Session projection and marks the board dirty for autosave', async () => {
    const harness = createPublicHarness({
      assertReady: async () => undefined,
      submit: async () => ({ taskId: 'task-1' }),
      resolve: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
    })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(
      harness.nodes.value.find((node) => node.id !== 'image-1')?.data.snapshotNode,
    ).toMatchObject({ sourceImageUrl: 'https://example.com/result.png', outputStatus: 'ready' })
    expect(harness.markDirty).toHaveBeenCalled()
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })
  })

  it('cancels a real reserved pending node when submit is cancelled', async () => {
    const harness = createPublicHarness({
      assertReady: async () => undefined,
      submit: async () => {
        throw new TaskCancelledError()
      },
      resolve: async () => ({ url: '', ratio: 1 }),
    })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1'])
  })

  it('does not remove a superseding pending node when an old submit resolves late', async () => {
    let resolveSubmit!: (value: { taskId: string }) => void
    const harness = createPublicHarness({
      assertReady: async () => undefined,
      submit: async () =>
        await new Promise((resolve) => {
          resolveSubmit = resolve
        }),
      resolve: async () => ({ url: '', ratio: 1 }),
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
    const core = createPendingImageExecutionCore()
    const graphSession = CanvasGraphSession.hydrate('board-a', createEmptyBoardSnapshot())
    core.attachBoard(
      BoardGraphSession.create('board-a', {
        graphSession,
      }),
    )
    const assertReady = vi.fn(async () => {
      throw new Error('not ready')
    })
    const execution = useImageWorkflowExecution({
      nodes: ref([]),
      edges: ref([]),
      readCanvasGraphStore: () => graphSession.store,
      rendererMetrics,
      boardBaseSnapshot: ref(createEmptyBoardSnapshot()),
      executionWorkspace: {
        state: { executionCore: core, isNodeRunning: () => false },
        commands: {
          runGeneration: vi.fn(),
          hydratePendingImageLeases: vi.fn(),
          reattachPendingImageResults: vi.fn(),
        },
      },
      cancelTasks: vi.fn(),
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
      taskTransport: { assertReady, submit: vi.fn(), resolve: vi.fn() },
    })
    await execution.executeImageWorkflow({
      kind: 'image-action',
      activationId: 'a',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1 },
      action: 'cutout',
      prompt: '',
      outputCount: 1,
      resultOptions: {},
    } as never)
    expect(assertReady).toHaveBeenCalledOnce()
    expect(core.snapshot().executions).toEqual([])
  })

  it('uses the complete outpaint request for readiness and submission', async () => {
    const assertReady = vi.fn<PendingImageWorkflowTaskTransport['assertReady']>(
      async () => undefined,
    )
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
      taskId: 'task-extend',
    }))
    const harness = createPublicHarness({
      assertReady,
      submit,
      resolve: async () => ({ url: 'https://example.com/extended.png', ratio: 0.75 }),
    })

    await harness.execution.executeImageWorkflow({
      kind: 'outpaint',
      activationId: 'extend',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1 },
      prompt: 'extend the horizon',
      outputCount: 1,
      frameRect: { left: -40, top: -10, width: 160, height: 140 },
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      targetRatio: 0.75,
      targetWidth: 320,
      resultOptions: {},
    } as never)

    const readinessRequest = assertReady.mock.calls[0]?.[0]
    expect(readinessRequest).toMatchObject({
      service: 'outpaint',
      image_urls: ['https://example.com/a.png'],
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
    expect(submit.mock.calls[0]?.[0]).toBe(readinessRequest)
  })

  it('uses the complete sized inpaint request for readiness and submission', async () => {
    const assertReady = vi.fn<PendingImageWorkflowTaskTransport['assertReady']>(
      async () => undefined,
    )
    const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
      taskId: 'task-inpaint',
    }))
    const harness = createPublicHarness({
      assertReady,
      submit,
      resolve: async () => ({ url: 'https://example.com/inpainted.png', ratio: 1 }),
    })

    await harness.execution.executeImageWorkflow({
      kind: 'inpaint',
      activationId: 'inpaint',
      sourceNodeId: 'image-1',
      source: { url: 'https://example.com/a.png', ratio: 1, mediaAssetId: 42 },
      prompt: 'remove the object',
      rawMaskDataUrl: 'data:image/png;base64,mask',
      outputCount: 1,
      resultOptions: {},
    } as never)

    const readinessRequest = assertReady.mock.calls[0]?.[0]
    expect(readinessRequest).toMatchObject({
      service: 'inpaint',
      prompt: 'remove the object',
      params: {
        output_count: 1,
        original_image_asset_id: 42,
        mask_asset_id: 'asset_mask1',
        original_image_width: 1200,
        original_image_height: 800,
        custom_width: 1200,
        custom_height: 800,
      },
    })
    expect(readinessRequest?.params).not.toHaveProperty('mask_image_url')
    expect(readinessRequest?.params).not.toHaveProperty('reference_image_urls')
    expect(submit.mock.calls[0]?.[0]).toBe(readinessRequest)
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
      taskTransport: { assertReady: vi.fn(), submit: vi.fn(), resolve: vi.fn() },
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
