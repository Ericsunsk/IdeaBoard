import type { Edge, Elements, Node } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type ImageNodeItem,
} from '../../board/domain'
import { i18n } from '../../../shared/i18n'
import { loadLocaleMessages } from '../../../shared/i18n/userLocale'
import { CanvasGraphSession } from '../graph-runtime'
import { useCanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { FlowNodeData } from '../types/flowTypes'
import { useCanvasGraphInteractions } from './useCanvasGraphInteractions'

function createImageSnapshot(): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 20,
    y: 30,
    width: 280,
    title: 'Domain image',
    sourceImageUrl: 'https://example.test/source.png',
    sourceMediaAssetId: 101,
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [
      {
        url: 'https://example.test/output.png',
        ratio: 0.75,
        mediaAssetId: 201,
        mediaRef: { assetId: 'asset_201', derivativeTier: 'preview' },
      },
    ],
  }
}

function createFlowImage(snapshot: ImageNodeItem): Node<FlowNodeData> {
  return {
    id: 'image-1',
    type: 'canvas',
    selected: true,
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Stale projection',
      snapshotNode: { ...snapshot, sourceImageUrl: 'https://stale.test/image.png' },
    },
  } as unknown as Node<FlowNodeData>
}

function createHarness() {
  const snapshot: BoardSnapshot = createEmptyBoardSnapshot()
  const image = createImageSnapshot()
  snapshot.imageNodes.push(image)
  const graphSession = CanvasGraphSession.hydrate('board-a', snapshot)
  const nodes = shallowRef<Node[]>([createFlowImage(image)])
  const edges = shallowRef<Edge[]>([])
  const selectedElements = shallowRef([nodes.value[0]] as unknown as Elements)
  const clear = vi.fn()
  const downloadImage = vi.fn()
  const openNodeImage = vi.fn(async () => undefined)
  const loading = { id: 'export-loading', destroy: vi.fn(), update: vi.fn() }
  const feedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn(),
    operationLoading: vi.fn(() => loading),
  }
  let attached = true
  const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const interactions = useCanvasGraphInteractions({
    nodes,
    edges,
    graphAdapter,
    readCanvasGraphStore: () => graphSession.store,
    isGraphSessionAttached: () => attached,
    selection: { selectedElements, clear },
    browser: { downloadImage },
    preview: { openNodeImage },
    feedback,
    isNodeExecutionActive: vi.fn(() => false),
  })
  return {
    interactions,
    nodes,
    selectedElements,
    clear,
    downloadImage,
    openNodeImage,
    feedback,
    loading,
    image,
    setAttached: (value: boolean) => {
      attached = value
    },
  }
}

describe('useCanvasGraphInteractions', () => {
  afterEach(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
  })

  it('enables image export when selection changes after the Graph Session attaches', () => {
    const harness = createHarness()
    const selectedImage = harness.nodes.value[0]

    harness.selectedElements.value = []
    harness.setAttached(false)
    expect(harness.interactions.viewModel.hasSelectedNodes.value).toBe(false)

    harness.setAttached(true)
    harness.selectedElements.value = [selectedImage] as unknown as Elements

    expect(harness.interactions.viewModel.hasSelectedNodes.value).toBe(true)
  })

  it('exports selected images from Graph facts through the browser adapter', async () => {
    const harness = createHarness()

    await harness.interactions.commands.exportSelectedImages()

    expect(harness.downloadImage).toHaveBeenCalledWith({
      url: 'https://example.test/output.png',
      assetId: 'asset_201',
      fileName: 'Domain image.png',
    })
    expect(harness.loading.destroy).toHaveBeenCalledOnce()
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledWith('Images queued: 1')
  })

  it('uses the active locale for image export feedback', async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
    const harness = createHarness()

    await harness.interactions.commands.exportSelectedImages()

    expect(harness.feedback.operationLoading).toHaveBeenCalledWith('正在导出 1 张图片…', {
      duration: Number.POSITIVE_INFINITY,
    })
    expect(harness.loading.update).toHaveBeenCalledWith('正在导出第 1/1 张图片…')
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledWith('已加入导出队列：1 张')
  })

  it('previews and appends images using Graph facts instead of stale Flow data', () => {
    const harness = createHarness()

    harness.interactions.commands.openImagePreview('image-1')
    harness.interactions.commands.appendImageToCanvas('image-1')

    expect(harness.openNodeImage).toHaveBeenCalledWith(harness.image, 'latest-output')
    expect(harness.nodes.value[1]?.data?.snapshotNode).toMatchObject({
      sourceImageUrl: 'https://example.test/output.png',
      sourceMediaAssetId: 101,
      sourceMediaRef: { assetId: 'asset_201', derivativeTier: 'preview' },
    })
  })

  it('duplicates selected image business data from Graph facts after Alt-drag', () => {
    const harness = createHarness()

    harness.interactions.commands.beginNodeDragDuplicateTracking(true)
    const draggedNode = harness.nodes.value[0]
    if (!draggedNode) throw new Error('expected dragged node')
    harness.nodes.value[0] = { ...draggedNode, position: { x: 50, y: 65 } }
    harness.interactions.commands.finishNodeDragDuplicateTracking(true)

    const copied = harness.nodes.value[1]?.data?.snapshotNode as ImageNodeItem
    expect(copied.sourceImageUrl).toBe('https://example.test/source.png')
    expect(copied.sourceMediaAssetId).toBe(101)
    expect(copied.outputImages).toEqual([
      { url: 'https://example.test/output.png', ratio: 0.75, mediaAssetId: 201 },
    ])
    expect(harness.nodes.value[0]?.position).toEqual({ x: 20, y: 30 })
  })

  it('owns selection clearing and Canvas interaction state', () => {
    const harness = createHarness()

    expect(harness.interactions.viewModel.hasSelectedNodes.value).toBe(true)
    harness.interactions.commands.clearSelection()
    harness.interactions.runtimePort.startInteraction('inpaint')

    expect(harness.clear).toHaveBeenCalledWith(harness.selectedElements.value)
    expect(harness.interactions.viewModel.isInteractionActive.value).toBe(true)
    expect(harness.interactions.viewModel.isInteractionLocked.value).toBe(true)

    harness.interactions.runtimePort.stopInteraction('inpaint')
    expect(harness.interactions.viewModel.isInteractionLocked.value).toBe(false)
  })

  it('returns an empty selection while the Graph Session is between Board attachments', () => {
    const harness = createHarness()
    harness.setAttached(false)

    expect(harness.interactions.viewModel.hasSelectedNodes.value).toBe(false)
  })
})
