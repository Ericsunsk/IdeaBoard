import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { useCanvasVueFlowGraphAdapter } from '../../composables/useCanvasVueFlowGraphAdapter'
import { useFlowImageWorkflowExecution } from './flowImageWorkflowExecution'

const executorMocks = vi.hoisted(() => ({
  assertTaskRequestReady: vi.fn(async () => undefined),
  submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
  waitExistingOrLiveTaskImageResults: vi.fn(async () => [
    {
      url: 'https://example.com/result.png',
      width: 400,
      height: 200,
    },
  ]),
}))

const imageSizeMocks = vi.hoisted(() => ({
  loadImageSizeFromUrl: vi.fn(async () => ({ width: 400, height: 400 })),
}))

vi.mock('../task-runner/flowImageDirectActionExecutor', () => executorMocks)
vi.mock('../media/imageSize', () => imageSizeMocks)
vi.mock('../../utils/animationFrame', () => ({
  waitForNextRenderFrame: vi.fn(async () => undefined),
}))

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeImageNode(nodeId: string): Node {
  const snapshot = makeImageSnapshot()
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode: snapshot,
    },
  }
}

function readImageSnapshot(nodes: Node[], nodeId: string): ImageNodeItem {
  const snapshot = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') throw new Error(`missing image node ${nodeId}`)
  return snapshot as ImageNodeItem
}

describe('flowImageWorkflowExecution', () => {
  beforeEach(() => {
    executorMocks.assertTaskRequestReady.mockClear()
    executorMocks.submitTaskRequest.mockClear()
    executorMocks.waitExistingOrLiveTaskImageResults.mockClear()
    imageSizeMocks.loadImageSizeFromUrl.mockClear()
  })

  it('executes image actions through the unified pending image batch path', async () => {
    const nodes: Ref<Node[]> = shallowRef([makeImageNode('image-source')])
    const edges: Ref<Edge[]> = shallowRef([])
    const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
    const flushAutosave = vi.fn(async () => undefined)
    const onSuccess = vi.fn()
    const onWarning = vi.fn()
    const execution = useFlowImageWorkflowExecution({
      nodes,
      edges,
      graphAdapter,
      flushAutosave,
      onSuccess,
      onWarning,
      getCurrentBoardId: () => 'board-a',
    })

    await execution.executeImageWorkflow({
      kind: 'image-action',
      sourceNodeId: 'image-source',
      source: {
        url: 'https://example.com/source.png',
        ratio: 1,
      },
      action: IMAGE_NODE_ACTION.removeBg,
      resultOptions: {
        anchor: {
          x: 0,
          y: 0,
          width: 280,
          outputAnchorNodeId: 'image-source',
        },
      },
    })

    expect(executorMocks.assertTaskRequestReady).toHaveBeenCalledTimes(1)
    expect(executorMocks.submitTaskRequest).toHaveBeenCalledTimes(1)
    expect(executorMocks.waitExistingOrLiveTaskImageResults).toHaveBeenCalledWith('task-1')
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(onSuccess).toHaveBeenCalledWith('处理完成')
    expect(onWarning).not.toHaveBeenCalled()

    const resultNode = nodes.value.find((node) => node.id !== 'image-source')
    expect(resultNode?.data?.kind).toBe('image')
    const snapshot = readImageSnapshot(nodes.value, resultNode!.id)
    expect(snapshot).toMatchObject({
      sourceImageUrl: 'https://example.com/result.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
      outputImages: [],
    })
    expect(snapshot.pendingTaskId).toBeUndefined()
  })
})
