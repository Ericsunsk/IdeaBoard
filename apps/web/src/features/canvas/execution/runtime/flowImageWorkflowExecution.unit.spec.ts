import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { TaskCancelledError } from '../../../../shared/api/taskEvents'
import { createEmptyCanvasBoardSnapshot } from '../../../canvas-board/domain'
import { useCanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions, readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import { CanvasGraphStore } from '../../graph-runtime'
import { canvasBoardSnapshotToFlow, flowToCanvasBoardSnapshot } from '../../utils/flowSnapshotAdapter'
import { CanvasBoardGraphSession, createPendingImageExecutionCore } from '../pending-execution'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'

vi.mock('../media/imageSize', () => ({ loadImageSizeFromUrl: vi.fn(async () => null) }))

function imageActionPlan() {
  return { kind: 'image-action', activationId: 'a', sourceNodeId: 'image-1', source: { url: 'https://example.com/a.png', ratio: 1 }, action: 'remove-bg', prompt: '', outputCount: 1, resultOptions: {} } as never
}

function createPublicHarness(transport: { assertReady: () => Promise<void>; submit: () => Promise<{ taskId: string }>; resolve: () => Promise<{ url: string; ratio: number }> }) {
  const base = createEmptyCanvasBoardSnapshot()
  base.imageNodes.push({ id: 1, type: 'image-node', x: 0, y: 0, width: 280, sourceImageUrl: 'https://example.com/a.png', sourceImageRatio: 1, outputStatus: 'ready', outputImages: [] })
  const flow = canvasBoardSnapshotToFlow(base)
  const nodes = ref(flow.nodes as never[])
  const edges = ref(flow.edges as never[])
  const markDirty = vi.fn()
  const flushAutosave = vi.fn(async () => undefined)
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes: nodes as never,
    edges: edges as never,
    afterCommand: (result) => {
      const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
      if (result.didChange && sideEffects?.markDirty !== false) {
        markDirty()
        void flushAutosave({ silent: true })
      }
    },
  })
  const core = createPendingImageExecutionCore()
  core.attachBoard(CanvasBoardGraphSession.create('board-a', {
    createGraphStore: () => CanvasGraphStore.fromSnapshot(flowToCanvasBoardSnapshot({ nodes: nodes.value, edges: edges.value }, base, { projection: 'runtime' })),
    onMaterialized: (store) => {
      const next = canvasBoardSnapshotToFlow(store.exportSnapshot(base))
      graphAdapter.replaceGraph({ nodes: next.nodes as never, edges: next.edges as never }, createFlowGraphCommandOptions({ recordHistory: false }))
    },
  }))
  const execution = useFlowImageWorkflowExecution({
    nodes: nodes as never, edges: edges as never, graphAdapter,
    createCurrentCanvasGraphStore: () => CanvasGraphStore.fromSnapshot(flowToCanvasBoardSnapshot({ nodes: nodes.value, edges: edges.value }, base, { projection: 'runtime' })),
    boardBaseSnapshot: ref(base),
    executionWorkspace: { state: { executionCore: core, isNodeRunning: (id) => core.captureActiveLeases([id]).length > 0 }, commands: { runPromptGeneration: vi.fn(), reattachPendingImageResults: vi.fn() } },
    cancelTasks: vi.fn(), onSuccess: vi.fn(), onWarning: vi.fn(), taskTransport: transport,
  })
  return { core, execution, flushAutosave, markDirty, nodes }
}

describe('useFlowImageWorkflowExecution', () => {
  it('materializes through Graph Session projection and marks the board dirty for autosave', async () => {
    const harness = createPublicHarness({ assertReady: async () => undefined, submit: async () => ({ taskId: 'task-1' }), resolve: async () => ({ url: 'https://example.com/result.png', ratio: 1 }) })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(harness.nodes.value.find((node) => node.id !== 'image-1')?.data.snapshotNode).toMatchObject({ sourceImageUrl: 'https://example.com/result.png', outputStatus: 'ready' })
    expect(harness.markDirty).toHaveBeenCalled()
    expect(harness.flushAutosave).toHaveBeenCalledWith({ silent: true })
  })

  it('cancels a real reserved pending node when submit is cancelled', async () => {
    const harness = createPublicHarness({ assertReady: async () => undefined, submit: async () => { throw new TaskCancelledError() }, resolve: async () => ({ url: '', ratio: 1 }) })
    await harness.execution.executeImageWorkflow(imageActionPlan())
    expect(harness.nodes.value.map((node) => node.id)).toEqual(['image-1'])
  })

  it('does not remove a superseding pending node when an old submit resolves late', async () => {
    let resolveSubmit!: (value: { taskId: string }) => void
    const harness = createPublicHarness({ assertReady: async () => undefined, submit: async () => await new Promise((resolve) => { resolveSubmit = resolve }), resolve: async () => ({ url: '', ratio: 1 }) })
    const running = harness.execution.executeImageWorkflow(imageActionPlan())
    await vi.waitFor(() => expect(harness.nodes.value).toHaveLength(2))
    const resultNodeId = harness.nodes.value.find((node) => node.id !== 'image-1')!.id as `image-${number}`
    const attachment = harness.core.currentAttachment()!
    const superseding = harness.core.begin(attachment, resultNodeId)
    if (superseding.kind !== 'started') throw new Error('Expected superseding execution')
    harness.core.markSubmitted(superseding.lease, 'task-new', 'action-task-new')
    resolveSubmit({ taskId: 'task-old' })
    await running
    expect(harness.nodes.value.some((node) => node.id === resultNodeId)).toBe(true)
    expect(harness.core.factFor(resultNodeId)).toMatchObject({ status: 'running', taskId: 'task-new' })
  })

  it('passes a prepared image action to the real Runner transport without per-run callbacks', async () => {
    const core = createPendingImageExecutionCore()
    const store = CanvasGraphStore.fromSnapshot(createEmptyCanvasBoardSnapshot())
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store }))
    const assertReady = vi.fn(async () => { throw new Error('not ready') })
    const execution = useFlowImageWorkflowExecution({
      nodes: ref([]), edges: ref([]), graphAdapter: {} as never, createCurrentCanvasGraphStore: () => store,
      boardBaseSnapshot: ref(createEmptyCanvasBoardSnapshot()),
      executionWorkspace: { state: { executionCore: core, isNodeRunning: () => false }, commands: { runPromptGeneration: vi.fn(), reattachPendingImageResults: vi.fn() } },
      cancelTasks: vi.fn(), onSuccess: vi.fn(), onWarning: vi.fn(),
      taskTransport: { assertReady, submit: vi.fn(), resolve: vi.fn() },
    })
    await execution.executeImageWorkflow({ kind: 'image-action', activationId: 'a', sourceNodeId: 'image-1', source: { url: 'https://example.com/a.png', ratio: 1 }, action: 'remove-bg', prompt: '', outputCount: 1, resultOptions: {} } as never)
    expect(assertReady).toHaveBeenCalledOnce()
    expect(core.snapshot().executions).toEqual([])
  })

  it('delegates prompt execution and pending reattach to the shared Core workspace seam', async () => {
    const core = createPendingImageExecutionCore()
    const store = CanvasGraphStore.fromSnapshot(createEmptyCanvasBoardSnapshot())
    core.attachBoard(CanvasBoardGraphSession.create('board-a', { createGraphStore: () => store }))
    const workspace = { state: { executionCore: core, isNodeRunning: () => false }, commands: { runPromptGeneration: vi.fn(), reattachPendingImageResults: vi.fn() } }
    const execution = useFlowImageWorkflowExecution({
      nodes: ref([]), edges: ref([]), graphAdapter: {} as never, createCurrentCanvasGraphStore: () => store,
      boardBaseSnapshot: ref(createEmptyCanvasBoardSnapshot()),
      executionWorkspace: workspace, cancelTasks: vi.fn(), onSuccess: vi.fn(), onWarning: vi.fn(),
      taskTransport: { assertReady: vi.fn(), submit: vi.fn(), resolve: vi.fn() },
    })
    const plan = { kind: 'prompt-generation', activationId: 'prompt', sourceNodeId: 'prompt-1', source: { url: '', ratio: 1 }, referenceImages: [], prompt: 'draw', outputCount: 1, aspectRatio: 'auto', resultOptions: {} } as never
    await execution.executeImageWorkflow(plan)
    await execution.reattachPendingImageResults()
    expect(workspace.commands.runPromptGeneration).toHaveBeenCalledWith(plan)
    expect(workspace.commands.reattachPendingImageResults).toHaveBeenCalledOnce()
  })
})
