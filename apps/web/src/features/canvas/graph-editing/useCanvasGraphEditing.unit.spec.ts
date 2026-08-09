import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../canvas-board/domain'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphAdapter,
} from '../composables/useCanvasVueFlowGraphAdapter'
import type { FlowNodeData } from '../types/flowTypes'
import { useCanvasGraphEditing } from './useCanvasGraphEditing'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../shared/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: mocks.prepareImageAssetForUpload,
}))

function createFlowNode(
  id: string,
  kind: FlowNodeData['kind'],
  snapshotNode: ImageNodeItem | PromptNodeItem,
): Node<FlowNodeData> {
  return {
    id,
    type: 'canvas',
    position: { x: snapshotNode.x, y: snapshotNode.y },
    data: {
      kind,
      nodeType: kind,
      title: snapshotNode.title ?? kind,
      snapshotNode,
    },
  }
}

function createHarness(initialNodes: Node[] = []) {
  const nodes = shallowRef<Node[]>(initialNodes)
  const edges = shallowRef<Edge[]>([])
  const pickedFile = ref<File | null>(null)
  const droppedFile = ref<File | null>(null)
  const upload = vi.fn(async () => ({
    url: 'https://example.com/uploaded.png',
    media_asset_id: 42,
    width: 200,
    height: 100,
  }))
  const openNodeImage = vi.fn(async () => {})
  const openImageUrl = vi.fn(async () => {})
  const feedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn(),
    operationLoading: vi.fn(),
  }
  const graphAdapter: CanvasVueFlowGraphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const editing = useCanvasGraphEditing({
    nodes,
    edges,
    boardBaseSnapshot: ref(createEmptyCanvasBoardSnapshot()),
    currentBoardId: ref('board-a'),
    graphAdapter,
    deleteNode: vi.fn(() => true),
    updateNodeInternals: vi.fn(),
    screenToFlowCoordinate: ({ x, y }) => ({ x: x + 10, y: y + 20 }),
    browser: {
      pickImageFile: async () => pickedFile.value,
      readDroppedFile: () => droppedFile.value,
      readDropScreenPoint: (event) => ({ x: event.clientX, y: event.clientY }),
      markDropAsCopy: vi.fn(),
    },
    upload: { upload },
    preview: { openNodeImage, openImageUrl },
    feedback,
  })

  return {
    editing,
    nodes,
    pickedFile,
    droppedFile,
    upload,
    openNodeImage,
    openImageUrl,
    feedback,
  }
}

describe('useCanvasGraphEditing', () => {
  beforeEach(() => {
    mocks.prepareImageAssetForUpload.mockImplementation(async (file: File) => ({
      originalFile: file,
      uploadFile: file,
      width: 200,
      height: 100,
      ratio: 0.5,
      dimensionsKnown: true,
      warnings: [],
      compressed: false,
    }))
  })

  it('creates Canvas Nodes through the canonical command seam', () => {
    const harness = createHarness()

    const nodeId = harness.editing.commands.createNodeAtPosition('prompt', 120, 80)

    expect(nodeId).toMatch(/^prompt-/)
    expect(harness.nodes.value).toEqual([
      expect.objectContaining({
        id: nodeId,
        position: { x: 120, y: 80 },
        data: expect.objectContaining({ kind: 'prompt' }),
      }),
    ])
  })

  it('owns file picking, upload, image mutation, and preview outcomes', async () => {
    const image: ImageNodeItem = {
      id: 1,
      type: 'image-node',
      x: 40,
      y: 60,
      width: 280,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputStatus: 'idle',
      outputImages: [],
    }
    const harness = createHarness([createFlowNode('image-1', 'image', image)])
    harness.pickedFile.value = new File(['image'], 'picked.png', { type: 'image/png' })
    const discardNodeEdits = vi.fn()
    const interactions = harness.editing.commands.createNodeInteractions({
      dismissTransientCanvasUi: vi.fn(),
      discardNodeEdits,
    })

    await interactions.onFlowImageOpenFile('image-1')
    harness.editing.commands.imageNodes.openPreview('image-1')

    const snapshot = harness.nodes.value[0]?.data?.snapshotNode as ImageNodeItem
    harness.editing.commands.imageNodes.appendToCanvas('image-1')
    expect(harness.upload).toHaveBeenCalledWith(expect.any(File), 'board-a')
    expect(snapshot).toMatchObject({
      title: 'picked.png',
      sourceImageUrl: 'https://example.com/uploaded.png',
      sourceMediaAssetId: 42,
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(discardNodeEdits).toHaveBeenCalledWith('image-1')
    expect(harness.openNodeImage).toHaveBeenCalledWith(snapshot, 'latest-output')
    expect(harness.nodes.value[1]?.data?.snapshotNode).toMatchObject({
      sourceImageUrl: 'https://example.com/uploaded.png',
      sourceMediaAssetId: 42,
    })
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledWith('Image 节点输入已更新')
  })

  it('normalizes Canvas drop and media-input drop through the browser adapter', async () => {
    const prompt: PromptNodeItem = {
      id: 1,
      type: 'prompt',
      x: 0,
      y: 0,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    }
    const harness = createHarness([createFlowNode('prompt-1', 'prompt', prompt)])
    harness.droppedFile.value = new File(['image'], 'dropped.png', { type: 'image/png' })
    const event = { clientX: 30, clientY: 40, preventDefault: vi.fn() } as unknown as DragEvent

    await harness.editing.commands.mediaInputs.drop('prompt-1', event)
    await harness.editing.commands.canvasFiles.drop(event)

    const promptSnapshot = harness.nodes.value.find((node) => node.id === 'prompt-1')?.data
      ?.snapshotNode as PromptNodeItem
    expect(promptSnapshot.inputImages).toEqual([
      expect.objectContaining({
        sourceType: 'external',
        url: 'https://example.com/uploaded.png',
        mediaAssetId: 42,
      }),
    ])
    expect(harness.nodes.value).toContainEqual(
      expect.objectContaining({
        position: { x: 40, y: 60 },
        data: expect.objectContaining({ kind: 'image' }),
      }),
    )
    expect(harness.upload).toHaveBeenCalledTimes(2)
  })
})
