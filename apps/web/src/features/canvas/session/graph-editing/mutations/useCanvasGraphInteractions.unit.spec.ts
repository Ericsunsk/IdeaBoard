import type { Edge, Elements, Node } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot, type BoardSnapshot, type ImageNodeItem } from '../../../../board'
import { i18n } from '../../../../../core/i18n'
import { loadLocaleMessages } from '../../../../../core/i18n/userLocale'
import { CanvasGraphSession } from '../../../core/graph'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { useCanvasGraphInteractions } from './useCanvasGraphInteractions'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'

beforeEach(() => {
  setMediaWorkspaceIdResolver(() => 'ws_test123')
})

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
    id: `image-${snapshot.id}`,
    type: 'canvas',
    isSelected: true,
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Stale projection',
      snapshotNode: { ...snapshot, sourceImageUrl: 'https://stale.test/image.png' },
    },
  } as unknown as Node<FlowNodeData>
}

function createHarness(images: ImageNodeItem[] = [createImageSnapshot()]) {
  const snapshot: BoardSnapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes.push(...images.map((image) => ({ ...image })))
  const graphSession = CanvasGraphSession.hydrate('board-a', snapshot)
  const flowNodes = images.map((image) => createFlowImage(image))
  const nodes = shallowRef<Node[]>(flowNodes)
  const edges = shallowRef<Edge[]>([])
  const selectedElements = shallowRef([...flowNodes] as unknown as Elements)
  const clear = vi.fn()
  const downloadImage = vi.fn()
  const openNodeImage = vi.fn(async () => undefined)
  const loading = { id: 'export-loading', destroy: vi.fn(), update: vi.fn() }
  const feedback = {
    operationSucceeded: vi.fn(),
    operationWarning: vi.fn(),
    operationLoading: vi.fn(() => loading),
  }
  let isAttached = true
  const interactions = useCanvasGraphInteractions({
    nodes,
    edges,
    readCanvasGraphStore: () => graphSession.store,
    isGraphSessionAttached: () => isAttached,
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
    image: images[0]!,
    graphSession,
    setAttached: (isValue: boolean) => {
      isAttached = isValue
    },
  }
}

describe('useCanvasGraphInteractions', () => {
  afterEach(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
    resetMediaWorkspaceIdResolver()
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
    const appended = harness.graphSession.store.exportSnapshot().imageNodes.at(-1)
    expect(appended).toMatchObject({
      sourceImageUrl: 'https://example.test/output.png',
      sourceMediaAssetId: 101,
      sourceMediaRef: { assetId: 'asset_201', derivativeTier: 'preview' },
    })
  })

  it('keeps the recipe when appending a baked node', () => {
    const baked: ImageNodeItem = {
      ...createImageSnapshot(),
      sourceImageUrl: 'https://example.test/baked.png',
      retouchRecipe: {
        version: 1,
        sourceAssetId: 'asset_src',
        plan: [{ kind: 'threshold', threshold: 128, invert: false }],
      },
    }
    const harness = createHarness([baked])

    harness.interactions.commands.appendImageToCanvas('image-1')

    const appended = harness.graphSession.store.exportSnapshot().imageNodes.at(-1)
    expect(appended?.retouchRecipe).toEqual(baked.retouchRecipe)
  })

  it('duplicates selected image business data from Graph facts after Alt-drag', () => {
    const harness = createHarness()

    harness.interactions.commands.beginNodeDragDuplicateTracking(true)
    const draggedNode = harness.nodes.value[0]
    if (!draggedNode) throw new Error('expected dragged node')
    harness.nodes.value[0] = { ...draggedNode, position: { x: 50, y: 65 } }
    harness.interactions.commands.finishNodeDragDuplicateTracking(true)

    const copied = harness.graphSession.store.exportSnapshot().imageNodes.at(-1) as ImageNodeItem
    expect(copied.sourceImageUrl).toBe('https://example.test/source.png')
    expect(copied.sourceMediaAssetId).toBe(101)
    expect(copied.outputImages).toEqual([
      { url: 'https://example.test/output.png', ratio: 0.75, mediaAssetId: 201 },
    ])
    const original = harness.graphSession.store
      .exportSnapshot()
      .imageNodes.find((node) => node.id === 1)
    expect(original?.x).toBe(20)
    expect(original?.y).toBe(30)
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

  it('supersedes an in-flight export when a second export starts', async () => {
    const secondImage: ImageNodeItem = {
      ...createImageSnapshot(),
      id: 2,
      title: 'Second image',
    }
    const harness = createHarness([createImageSnapshot(), secondImage])
    const resolvers: Array<(value: unknown) => void> = []
    harness.downloadImage.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvers.push(resolve as (value: unknown) => void)
        }),
    )

    const first = harness.interactions.commands.exportSelectedImages()
    // Let the first run reach its first download await.
    await Promise.resolve()
    await Promise.resolve()
    expect(harness.downloadImage).toHaveBeenCalledTimes(1)

    const second = harness.interactions.commands.exportSelectedImages()
    await Promise.resolve()
    await Promise.resolve()
    expect(harness.downloadImage).toHaveBeenCalledTimes(2)

    // Drain every pending download until both runs settle.
    for (let i = 0; i < 10; i += 1) {
      resolvers.splice(0).forEach((resolve) => resolve(undefined))
      await Promise.resolve()
      await new Promise((resolve) => setTimeout(resolve, 0))
      if (harness.downloadImage.mock.calls.length >= 3) {
        resolvers.splice(0).forEach((resolve) => resolve(undefined))
        await Promise.resolve()
        await new Promise((resolve) => setTimeout(resolve, 0))
        break
      }
    }
    await first
    await second

    // The superseded run downloaded once, then bailed: only the latest run
    // completes all entries and reports success.
    expect(harness.downloadImage).toHaveBeenCalledTimes(3)
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledTimes(1)
    expect(harness.feedback.operationSucceeded).toHaveBeenCalledWith('Images queued: 2')
    // First handle destroyed by the second run's start, second handle
    // destroyed by its own finally — the bailed run destroys nothing.
    expect(harness.loading.destroy).toHaveBeenCalledTimes(2)
  })
})
