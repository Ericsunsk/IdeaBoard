import type { Edge, Elements, Node } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import { createCanvasGraphStore } from '../graph-runtime'
import { useCanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
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
    outputImages: [{ url: 'https://example.test/output.png', ratio: 0.75, mediaAssetId: 201 }],
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
  const snapshot: CanvasBoardSnapshot = createEmptyCanvasBoardSnapshot()
  const image = createImageSnapshot()
  snapshot.imageNodes.push(image)
  const nodes = shallowRef<Node[]>([createFlowImage(image)])
  const edges = shallowRef<Edge[]>([])
  const selectedElements = shallowRef([nodes.value[0]] as unknown as Elements)
  const clear = vi.fn()
  const downloadImage = vi.fn()
  const openNodeImage = vi.fn(async () => undefined)
  const loading = { id: 'export-loading', destroy: vi.fn() }
  const feedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn(),
    operationLoading: vi.fn(() => loading),
  }
  const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const interactions = useCanvasGraphInteractions({
    nodes,
    edges,
    graphAdapter,
    createCurrentCanvasGraphStore: () => createCanvasGraphStore(snapshot),
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
  }
}

describe('useCanvasGraphInteractions', () => {
  it('exports selected images from Graph facts through the browser adapter', async () => {
    const harness = createHarness()

    await harness.interactions.commands.exportSelectedImages()

    expect(harness.downloadImage).toHaveBeenCalledWith({
      url: 'https://example.test/output.png',
      fileName: 'Domain image.png',
    })
    expect(harness.loading.destroy).toHaveBeenCalledOnce()
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledWith('Export queued: 1 image')
  })

  it('previews and appends images using Graph facts instead of stale Flow data', () => {
    const harness = createHarness()

    harness.interactions.commands.openImagePreview('image-1')
    harness.interactions.commands.appendImageToCanvas('image-1')

    expect(harness.openNodeImage).toHaveBeenCalledWith(harness.image, 'latest-output')
    expect(harness.nodes.value[1]?.data?.snapshotNode).toMatchObject({
      sourceImageUrl: 'https://example.test/output.png',
      sourceMediaAssetId: 201,
    })
  })

  it('duplicates selected image business data from Graph facts after Alt-drag', () => {
    const harness = createHarness()

    harness.interactions.commands.beginNodeDragDuplicateTracking(true)
    harness.nodes.value[0] = { ...harness.nodes.value[0], position: { x: 50, y: 65 } }
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
    harness.interactions.runtimePort.startInteraction('image-mask-editor')

    expect(harness.clear).toHaveBeenCalledWith(harness.selectedElements.value)
    expect(harness.interactions.viewModel.isInteractionActive.value).toBe(true)
    expect(harness.interactions.viewModel.isInteractionLocked.value).toBe(true)

    harness.interactions.runtimePort.stopInteraction('image-mask-editor')
    expect(harness.interactions.viewModel.isInteractionLocked.value).toBe(false)
  })
})
