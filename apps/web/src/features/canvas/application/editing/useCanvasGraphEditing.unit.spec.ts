import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createEmptyBoardSnapshot,
  resolveImageNodeStageHeight,
  type CanvasNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board/domain'
import { CanvasGraphSession } from '../../domain/graph'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphAdapter,
} from '../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import { bindCanvasGraphSessionProjection } from '../../infrastructure/vue-flow-adapter/bindCanvasGraphSessionProjection'
import { boardSnapshotToFlow } from '../../infrastructure/vue-flow-adapter/flowSnapshotAdapter.ts'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
import { useCanvasGraphEditing } from './useCanvasGraphEditing'
import {
  writeCanvasLibraryImportTransfer,
  type CanvasLibraryImportDescriptor,
} from '../../../library'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../../shared/media/imageAssetPipeline', () => ({
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
    asset_id: 'asset_42',
    width: 200,
    height: 100,
  }))
  const openImageUrl = vi.fn(async () => {})
  const feedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn(),
    operationLoading: vi.fn(),
  }
  const graphAdapter: CanvasVueFlowGraphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const graphSnapshot = createEmptyBoardSnapshot()
  for (const node of initialNodes) {
    const snapshotNode = node.data?.snapshotNode as CanvasNodeItem | undefined
    if (!snapshotNode) continue
    if (snapshotNode.type === 'image-node') graphSnapshot.imageNodes.push(snapshotNode)
    else if (snapshotNode.type === 'prompt') graphSnapshot.promptNodes.push(snapshotNode)
    else graphSnapshot.fusionNodes.push(snapshotNode)
  }
  const graphSession = CanvasGraphSession.hydrate('board-a', graphSnapshot)
  bindCanvasGraphSessionProjection({
    session: graphSession,
    graphAdapter,
    readCurrent: () => ({ nodes: nodes.value, edges: edges.value }),
    projectFull: () => {
      const flow = boardSnapshotToFlow(graphSession.exportSnapshot(graphSnapshot))
      return { nodes: flow.nodes, edges: flow.edges }
    },
    isCurrentAttachment: () => true,
  })
  const materializeLibraryAsset = vi.fn(async () => ({
    mediaAssetId: 43,
    url: 'https://example.com/materialized.png',
    width: 200,
    height: 100,
  }))
  const libraryImportAttachment = {} as import('../execution/board-attachment').BoardAttachment
  const markDropAsCopy = vi.fn()
  const editing = useCanvasGraphEditing({
    nodes,
    edges,
    boardBaseSnapshot: ref(graphSnapshot),
    currentBoardId: ref('board-a'),
    graphAdapter,
    readCanvasGraphStore: () => graphSession.store,
    deleteNode: vi.fn(() => true),
    updateNodeInternals: vi.fn(),
    screenToFlowCoordinate: ({ x, y }) => ({ x: x + 10, y: y + 20 }),
    browser: {
      pickImageFile: async () => pickedFile.value,
      readDroppedFile: () => droppedFile.value,
      readDropScreenPoint: (event) => ({ x: event.clientX, y: event.clientY }),
      markDropAsCopy,
    },
    upload: { upload },
    materializeLibraryAsset,
    readLibraryImportAttachment: () => libraryImportAttachment,
    selectOnlyNode: vi.fn(() => true),
    preview: { openImageUrl },
    feedback,
  })

  return {
    editing,
    nodes,
    pickedFile,
    droppedFile,
    upload,
    materializeLibraryAsset,
    markDropAsCopy,
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

  it('owns file picking, upload, and image mutation outcomes', async () => {
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
    const snapshot = harness.nodes.value[0]?.data?.snapshotNode as ImageNodeItem
    expect(harness.upload).toHaveBeenCalledWith(expect.any(File), 'board-a')
    expect(snapshot).toMatchObject({
      title: 'picked.png',
      sourceImageUrl: '/api/user/media-assets/asset_42/derivatives/preview/content',
      sourceMediaRef: {
        assetId: 'asset_42',
        derivativeTier: 'preview',
      },
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
    })
    expect(discardNodeEdits).toHaveBeenCalledWith('image-1')
    expect(harness.feedback.operationSucceeded).not.toHaveBeenCalled()
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
        url: '/api/user/media-assets/asset_42/derivatives/preview/content',
        mediaRef: {
          assetId: 'asset_42',
          derivativeTier: 'preview',
        },
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

  it('imports a dragged Library asset at its Canvas drop coordinate without upload', async () => {
    const harness = createHarness()
    const values = new Map<string, string>()
    const types: string[] = []
    const dataTransfer = {
      types,
      effectAllowed: 'all',
      dropEffect: 'none',
      setData(type: string, value: string) {
        values.set(type, value)
        types.push(type)
      },
      getData(type: string) {
        return values.get(type) ?? ''
      },
      files: [new File(['ignored'], 'should-not-upload.png', { type: 'image/png' })],
    } as unknown as DataTransfer
    const asset: CanvasLibraryImportDescriptor = {
      assetId: 42,
      previewUrl: 'https://images.unsplash.com/dragged-asset-preview',
      title: 'Dragged asset',
      width: 1200,
      height: 800,
    }
    writeCanvasLibraryImportTransfer(dataTransfer, asset)

    await harness.editing.commands.canvasFiles.drop({
      clientX: 90,
      clientY: 110,
      dataTransfer,
    } as unknown as DragEvent)
    await Promise.resolve()

    expect(harness.materializeLibraryAsset).toHaveBeenCalledWith(42)
    expect(harness.upload).not.toHaveBeenCalled()
    expect(harness.nodes.value).toContainEqual(
      expect.objectContaining({
        position: { x: 100, y: 130 },
        data: expect.objectContaining({
          kind: 'image',
          snapshotNode: expect.objectContaining({
            sourceImageUrl: 'https://example.com/materialized.png',
            sourceMediaAssetId: 43,
          }),
        }),
      }),
    )

    harness.editing.commands.canvasFiles.dragOver({ dataTransfer } as unknown as DragEvent)
    expect(harness.markDropAsCopy).toHaveBeenCalled()
  })

  it('creates a running pending retouch node matching the source node dimensions and ratio', () => {
    const harness = createHarness()
    const sourceNode: ImageNodeItem = {
      id: 10,
      type: 'image-node',
      x: 100,
      y: 200,
      width: 460,
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 1.5,
      outputStatus: 'ready',
      outputImages: [],
    }

    const createdNodeId = harness.editing.workflowPort.createPendingRetouchNode(sourceNode)

    expect(createdNodeId).toMatch(/^image-\d+$/)
    expect(harness.nodes.value).toContainEqual(
      expect.objectContaining({
        id: createdNodeId,
        position: { x: 660, y: 200 },
        data: expect.objectContaining({
          kind: 'image',
          snapshotNode: expect.objectContaining({
            width: 460,
            sourceImageRatio: 1.5,
            outputStatus: 'running',
            outputImages: [],
          }),
        }),
      }),
    )
  })

  it('places a Retouch result below an image occupying the preferred result position', () => {
    const sourceNode: ImageNodeItem = {
      id: 10,
      type: 'image-node',
      x: 100,
      y: 200,
      width: 460,
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 1.5,
      outputStatus: 'ready',
      outputImages: [],
    }
    const variationNode: ImageNodeItem = {
      id: 11,
      type: 'image-node',
      x: 660,
      y: 200,
      width: 460,
      sourceImageUrl: 'https://example.com/variation.png',
      sourceImageRatio: 1.5,
      outputStatus: 'ready',
      outputImages: [],
    }
    const harness = createHarness([
      createFlowNode('image-10', 'image', sourceNode),
      createFlowNode('image-11', 'image', variationNode),
    ])

    const createdNodeId = harness.editing.workflowPort.createPendingRetouchNode(sourceNode)
    const retouchNode = harness.nodes.value.find((node) => node.id === createdNodeId)
    const variationHeight = 26 + resolveImageNodeStageHeight(460, 1.5, true)

    expect(retouchNode).toEqual(
      expect.objectContaining({
        position: {
          x: 660,
          y: 200 + variationHeight + 14,
        },
      }),
    )
  })
})
