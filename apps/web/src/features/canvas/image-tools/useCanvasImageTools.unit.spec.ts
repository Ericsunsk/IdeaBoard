import type { Node } from '@vue-flow/core'
import { createApp, defineComponent, nextTick, ref, shallowRef, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import { renderRetouchLayerStackFile } from '../retouch/services/imageAdjustmentRenderer'
import { IMAGE_STRENGTH_ACTIONS, type ImageStrengthAction } from '../execution'
import type { UseCanvasImageToolsOptions } from './imageToolSession'
import { useCanvasImageTools, type CanvasImageTools } from '.'
import { i18n } from '../../../shared/i18n'
import { loadLocaleMessages } from '../../../shared/i18n/userLocale'

vi.mock('../retouch/services/imageAdjustmentRenderer', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../retouch/services/imageAdjustmentRenderer')>()
  return {
    ...actual,
    renderRetouchLayerStackFile: vi.fn(),
  }
})

vi.mock('../retouch/useImageAdjustmentRenderedPreview', async () => {
  const { computed } = await import('vue')
  return {
    useImageAdjustmentRenderedPreview: vi.fn(() => ({
      retouchRenderedPreview: computed(() => null),
      retouchPreviewSource: computed(() => null),
    })),
  }
})

vi.mock('../inpaint/lib/maskPaintingCanvas', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../inpaint/lib/maskPaintingCanvas')>()
  return {
    ...actual,
    buildMaskCommitResult: vi.fn(() => ({
      executionMaskDataUrl: 'data:image/png;base64,execution-mask',
      maskDataUrl: 'data:image/png;base64,preview-mask',
    })),
  }
})

const renderLayerStackFileMock = vi.mocked(renderRetouchLayerStackFile)

const mediaAssetRequestMock = vi.hoisted(() => vi.fn())
vi.mock('../../../shared/api/client', () => ({
  api: { request: mediaAssetRequestMock },
}))
vi.mock('../../../shared/api/userEventStream', () => ({
  subscribeUserEvent: vi.fn(() => vi.fn()),
}))

const imageNodeFixtures: Record<
  string,
  {
    display: { url: string; ratio: number; mediaAssetId?: number } | null
    snapshot: ImageNodeItem
  }
> = {
  'image-1': {
    display: { url: 'https://example.com/source.png', ratio: 1 },
    snapshot: {
      id: 1,
      type: 'image-node',
      title: 'Source',
      x: 10,
      y: 20,
      width: 300,
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 1,
      outputCount: 1,
      outputStatus: 'ready',
      outputImages: [],
    },
  },
  'image-2': {
    display: { url: 'https://example.com/current.png', ratio: 1 },
    snapshot: {
      id: 2,
      type: 'image-node',
      title: 'Current',
      x: 20,
      y: 30,
      width: 280,
      sourceImageUrl: 'https://example.com/current.png',
      sourceImageRatio: 1,
      outputCount: 1,
      outputStatus: 'ready',
      outputImages: [],
    },
  },
  'legacy-image': {
    display: { url: 'https://example.com/legacy.png', ratio: 1 },
    snapshot: {
      id: 3,
      type: 'image-node',
      title: 'Legacy',
      x: 30,
      y: 40,
      width: 260,
      sourceImageUrl: 'https://example.com/legacy.png',
      sourceImageRatio: 1,
      outputCount: 1,
      outputStatus: 'ready',
      outputImages: [],
    },
  },
  'empty-image': {
    display: null,
    snapshot: {
      id: 4,
      type: 'image-node',
      title: 'Empty',
      x: 40,
      y: 50,
      width: 280,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputCount: 1,
      outputStatus: 'idle',
      outputImages: [],
    },
  },
  'image-5': {
    display: { url: '/api/user/media-assets/3/original/content', ratio: 1, mediaAssetId: 3 },
    snapshot: {
      id: 5,
      type: 'image-node',
      title: 'Library image',
      x: 50,
      y: 60,
      width: 280,
      sourceImageUrl: 'https://cdn.example.com/library-preview.jpg',
      sourceMediaAssetId: 3,
      sourceImageRatio: 1,
      outputCount: 1,
      outputStatus: 'ready',
      outputImages: [],
    },
  },
}

function createStageElement() {
  const stage = document.createElement('div')
  Object.defineProperties(stage, {
    clientWidth: { value: 320 },
    clientHeight: { value: 240 },
  })
  return stage
}

function mountCanvasImageTools() {
  const stage = createStageElement()
  const activeTool = ref('select')
  const canvasWrapRef = ref<HTMLElement | null>(null)
  const isCanvasInteractionLocked = ref(false)
  const currentBoardId = ref('board-1')
  const generationAvailable = ref(true)
  const directActionRunningNodeIds = ref<Set<string>>(new Set())
  const nodes: Ref<Node[]> = shallowRef(
    Object.keys(imageNodeFixtures).map((id) => ({ id, data: { kind: 'image' } })) as Node[],
  )
  const connectionAdapter = { connectIntent: vi.fn(() => true) }
  const createPendingAdjustmentNode = vi.fn<
    NonNullable<UseCanvasImageToolsOptions['createPendingAdjustmentNode']>
  >(() => 'image-3')
  const applyImageFileToImageNode = vi.fn<
    NonNullable<UseCanvasImageToolsOptions['applyImageFileToImageNode']>
  >(async () => true)
  const removeNode = vi.fn<NonNullable<UseCanvasImageToolsOptions['removeNode']>>()
  const setViewport = vi.fn()
  const setCenter = vi.fn()
  const runInpaintAction = vi.fn()
  const activeStrengthAction = ref<ImageStrengthAction | null>(null)
  const feedback = {
    warning: vi.fn<(message: string) => void>(),
    succeeded: vi.fn<(message: string) => void>(),
    actionUnavailable: vi.fn<(message: string) => void>(),
  }
  let runtime!: CanvasImageTools
  const host = document.createElement('div')
  canvasWrapRef.value = host
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        runtime = useCanvasImageTools({
          nodes,
          activeTool,
          canvasWrapRef,
          isCanvasInteractionLocked,
          currentBoardId,
          getImageNodeDisplayPayload: (nodeId) => imageNodeFixtures[nodeId]?.display ?? null,
          getImageNodeSnapshot: (nodeId): ImageNodeItem | null =>
            imageNodeFixtures[nodeId]?.snapshot ?? null,
          getImageStageElement: (nodeId) => (imageNodeFixtures[nodeId] ? stage : null),
          screenToFlowCoordinate: (point) => point,
          getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
          setViewport,
          setCenter,
          createPendingAdjustmentNode,
          applyImageFileToImageNode,
          removeNode,
          connectionAdapter,
          uploadImageToBoard: async () => ({ url: 'https://example.com/ref.png', ratio: 1 }),
          openImageUrlPreview: vi.fn(),
          clearActiveNodePanel: vi.fn(),
          hideStrengthControl: vi.fn(),
          runInpaintAction,
          isImageActionPanelVisible: (nodeId) => nodeId === 'image-1',
          isImagePromptComposerVisible: (nodeId) => nodeId === 'image-1',
          isGenerationAvailable: () => generationAvailable.value,
          isNodeExecutionActive: (nodeId) => directActionRunningNodeIds.value.has(nodeId),
          strengthControl: {
            activeAction: activeStrengthAction,
            getValue: () => 50,
            setRangeRef: vi.fn(),
            setTriggerRef: vi.fn(),
            onRangePointerDown: vi.fn(),
            onInput: vi.fn(),
            onChange: vi.fn(),
            onPointerUp: vi.fn(),
            onClick: vi.fn(),
          },
          clearSelectedFlowElements: vi.fn(),
          startCanvasInteraction: vi.fn(),
          stopCanvasInteraction: vi.fn(),
          feedback,
        })
        return () => null
      },
    }),
  )
  app.mount(host)

  return {
    activeTool,
    activeStrengthAction,
    app,
    connectionAdapter,
    createPendingAdjustmentNode,
    applyImageFileToImageNode,
    removeNode,
    currentBoardId,
    directActionRunningNodeIds,
    generationAvailable,
    host,
    isCanvasInteractionLocked,
    runInpaintAction,
    feedback,
    runtime,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function prepareRenderableAdjustmentSession(runtime: CanvasImageTools) {
  expect(runtime.commands.lifecycle.enter('image-1', 'adjust')).toBe(true)
  runtime.commands.chrome.selectAdjustmentLayer('threshold')
  runtime.commands.chrome.updateEditorAdjustmentParams({ threshold: 128, invert: true })
  return runtime.viewModel.chrome.value.editor.layers
}

async function flushPromises() {
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  i18n.global.locale.value = 'en'
  vi.restoreAllMocks()
  renderLayerStackFileMock.mockReset()
  mediaAssetRequestMock.mockReset()
})

describe('useCanvasImageTools image adjustment final apply', () => {
  it('releases the presentation session across no-tool, mode switch, node switch, reset, and unmount', () => {
    const harness = mountCanvasImageTools()
    const session = harness.runtime.viewModel.nodeBindings.stageFor('image-1').session
    const canvas = document.createElement('canvas')
    try {
      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'adjust')).toBe(true)
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)
      session.inpaint.setMaskCanvasRef('image-1', canvas)
      expect(harness.runtime.commands.lifecycle.enter('image-2', 'inpaint')).toBe(true)
      expect(harness.runtime.viewModel.active.nodeId.value).toBe('image-2')
      harness.runtime.commands.lifecycle.cancel()
      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
      expect(harness.runtime.viewModel.active.nodeId.value).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('exposes image tools through viewModel and commands buckets only', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(Object.keys(harness.runtime).sort()).toEqual(['commands', 'viewModel'])
      expect(harness.runtime.viewModel.active.nodeId.value).toBeNull()
      expect(harness.runtime.viewModel.active.maskNodeId.value).toBeNull()
      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
      expect(harness.runtime.viewModel.locks.isChromeHidden.value).toBe(false)
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(false)
      expect(harness.runtime.viewModel.chrome.value.isReady).toBe(false)
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
      expect(harness.runtime.viewModel.nodeBindings.stageFor).toEqual(expect.any(Function))
      expect(harness.runtime.viewModel.nodeBindings.actionFor).toEqual(expect.any(Function))
      expect(harness.runtime.commands.lifecycle.enter).toEqual(expect.any(Function))
      expect(harness.runtime.commands.lifecycle.cancel).toEqual(expect.any(Function))
      expect(harness.runtime.commands.lifecycle.commit).toEqual(expect.any(Function))
      expect(harness.runtime.commands.lifecycle.discardNodeEdits).toEqual(expect.any(Function))
      expect(harness.runtime.commands.editor.apply).toEqual(expect.any(Function))
      expect(harness.runtime.commands.editor.onCanvasWheel).toEqual(expect.any(Function))
      expect(harness.runtime.commands.editor.focusAndShowChrome).toEqual(expect.any(Function))
      expect(harness.runtime.commands.editor.scheduleChromeAnchorUpdate).toEqual(
        expect.any(Function),
      )
      expect(harness.runtime.commands.chrome.selectAdjustmentLayer).toEqual(expect.any(Function))
    } finally {
      harness.unmount()
    }
  })

  it('locks canvas only when a lock-level interaction or editor session is active', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(false)

      harness.isCanvasInteractionLocked.value = true
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(true)

      harness.isCanvasInteractionLocked.value = false
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(false)

      expect(harness.runtime.commands.lifecycle.enter('image-1', 'adjust')).toBe(true)

      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('exits editor mode immediately while the derived node is pending', async () => {
    let resolveRender!: (file: File) => void
    renderLayerStackFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((resolve) => {
          options?.onProgress?.({ phase: 'render', loaded: 24, total: 96 })
          resolveRender = resolve
        }),
    )
    const harness = mountCanvasImageTools()

    try {
      const renderableLayers = prepareRenderableAdjustmentSession(harness.runtime)

      const apply = harness.runtime.commands.editor.apply()
      await flushPromises()

      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
      expect(harness.runtime.viewModel.active.nodeId.value).toBeNull()
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(true)
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(true)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-3').directGenerating).toBe(true)
      expect(harness.createPendingAdjustmentNode).toHaveBeenCalledWith(
        expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
        expect.objectContaining({ id: 1 }),
      )
      expect(harness.connectionAdapter.connectIntent).toHaveBeenCalledWith({
        sourceNodeId: 'image-1',
        sourcePort: 'image',
        targetNodeId: 'image-3',
        targetPort: 'image',
      })
      expect(renderLayerStackFileMock).toHaveBeenCalledWith(
        { url: 'https://example.com/source.png', title: 'Source' },
        renderableLayers,
        expect.objectContaining({
          scopeId: 'image-1',
        }),
      )

      resolveRender(new File(['rendered'], 'rendered.webp', { type: 'image/webp' }))
      await apply

      expect(harness.applyImageFileToImageNode).toHaveBeenCalledWith('image-3', expect.any(File))
      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
      expect(harness.runtime.viewModel.locks.isCanvasLocked.value).toBe(false)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-3').directGenerating).toBe(false)
      expect(harness.feedback.warning).not.toHaveBeenCalled()
      expect(harness.feedback.succeeded).toHaveBeenCalledWith('Image generated')
      expect(harness.feedback.succeeded).toHaveBeenCalledTimes(1)
    } finally {
      harness.unmount()
    }
  })

  it('renders materialized Library images through the same-origin processing derivative', async () => {
    mediaAssetRequestMock.mockResolvedValue({
      code: 0,
      msg: '',
      data: {
        asset_id: 'asset-3',
        status: 'ready',
        mime_type: 'image/jpeg',
        width: 2048,
        height: 1024,
        size_bytes: 100,
        original: { status: 'ready', url: '/api/original.jpg' },
        derivatives: {
          processing: { status: 'ready', format: 'webp', url: '/api/processing.webp' },
        },
      },
    })
    renderLayerStackFileMock.mockResolvedValue(
      new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    )
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-5', 'adjust')).toBe(true)
      harness.runtime.commands.chrome.selectAdjustmentLayer('threshold')
      harness.runtime.commands.chrome.updateEditorAdjustmentParams({ threshold: 128, invert: true })

      await harness.runtime.commands.editor.apply()

      expect(renderLayerStackFileMock).toHaveBeenCalledWith(
        {
          url: '/api/user/media-assets/3/derivatives/processing/webp/content',
          title: 'Library image',
        },
        expect.any(Array),
        expect.objectContaining({ scopeId: 'image-5' }),
      )
      expect(harness.feedback.succeeded).toHaveBeenCalledWith('Image generated')
    } finally {
      harness.unmount()
    }
  })

  it('reports one generation failure when the rendered image cannot be materialized', async () => {
    renderLayerStackFileMock.mockResolvedValue(
      new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    )
    const harness = mountCanvasImageTools()
    harness.applyImageFileToImageNode.mockResolvedValueOnce(false)

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.feedback.warning).toHaveBeenCalledWith('Image generation failed')
      expect(harness.feedback.warning).toHaveBeenCalledTimes(1)
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.createPendingAdjustmentNode).toHaveBeenCalledWith(
        expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
        expect.objectContaining({ id: 1 }),
      )
      expect(harness.connectionAdapter.connectIntent).toHaveBeenCalledWith({
        sourceNodeId: 'image-1',
        sourcePort: 'image',
        targetNodeId: 'image-3',
        targetPort: 'image',
      })
      expect(harness.runtime.viewModel.active.mode.value).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('removes the pending node when automatic connection is rejected', async () => {
    renderLayerStackFileMock.mockResolvedValue(
      new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    )
    const harness = mountCanvasImageTools()
    harness.connectionAdapter.connectIntent.mockReturnValue(false)

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.removeNode).toHaveBeenCalledWith('image-3')
      expect(harness.feedback.warning).toHaveBeenCalledWith(
        'Image created, but automatic connection failed. Please connect it manually.',
      )
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
    } finally {
      harness.unmount()
    }
  })

  it('clears applying state when pending node creation fails synchronously', async () => {
    const harness = mountCanvasImageTools()
    harness.createPendingAdjustmentNode.mockImplementation(() => {
      throw new Error('graph write failed')
    })

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
      expect(harness.feedback.warning).toHaveBeenCalledWith('Image generation failed')
    } finally {
      harness.unmount()
    }
  })

  it('reports one generation failure when rendering adjustments fails', async () => {
    renderLayerStackFileMock.mockRejectedValueOnce(new Error('render failed'))
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.feedback.warning).toHaveBeenCalledWith('Image generation failed')
      expect(harness.feedback.warning).toHaveBeenCalledTimes(1)
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('uses the active locale for image adjustment terminal feedback', async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
    renderLayerStackFileMock.mockRejectedValueOnce(new Error('render failed'))
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.feedback.warning).toHaveBeenCalledOnce()
      expect(harness.feedback.warning).toHaveBeenCalledWith('图片生成失败')
    } finally {
      harness.unmount()
    }
  })

  it('does not render or create a derived node from a noncanonical active source', async () => {
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)
      expect(harness.runtime.commands.lifecycle.enter('legacy-image', 'adjust')).toBe(true)
      harness.runtime.commands.chrome.selectAdjustmentLayer('threshold')

      await harness.runtime.commands.editor.apply()

      expect(renderLayerStackFileMock).not.toHaveBeenCalled()
      expect(harness.connectionAdapter.connectIntent).not.toHaveBeenCalled()
      expect(harness.feedback.warning).toHaveBeenCalledWith(
        'Failed to write the image node to the canvas.',
      )
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('does not write an old final render result back after switching to another board', async () => {
    let resolveOldRender!: (file: File) => void
    renderLayerStackFileMock.mockImplementation(
      async (_source, _layers): Promise<File> =>
        new Promise((resolve) => {
          resolveOldRender = resolve
        }),
    )
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      const oldApply = harness.runtime.commands.editor.apply()
      await flushPromises()

      // Switch to a different board before render completes
      harness.currentBoardId.value = 'board-other'

      resolveOldRender(new File(['old'], 'old.webp', { type: 'image/webp' }))
      await oldApply

      expect(harness.applyImageFileToImageNode).not.toHaveBeenCalled()
      expect(harness.removeNode).toHaveBeenCalledWith('image-3')
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.feedback.warning).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('exposes typed stage and action bindings for the active image node', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      const stageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const actionBinding = harness.runtime.viewModel.nodeBindings.actionFor('image-1')

      expect(stageBinding.active).toBe(true)
      expect(stageBinding.mode).toBe('inpaint')
      expect(stageBinding.nodeId).toBe('image-1')
      expect(stageBinding.session.inpaint.inpaintOverlayStyle.value).toEqual({
        left: '40px',
        top: '0px',
        width: '240px',
        height: '240px',
      })
      expect(actionBinding.actionMenuVisible).toBe(true)
      expect(actionBinding.promptPlaceholder).toBe('Optional instructions')
    } finally {
      harness.unmount()
    }
  })

  it('keeps node binding identities stable when state is unchanged', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      const stageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const actionBinding = harness.runtime.viewModel.nodeBindings.actionFor('image-1')
      const inactiveStageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-2')
      const inactiveActionBinding = harness.runtime.viewModel.nodeBindings.actionFor('image-2')

      expect(harness.runtime.viewModel.nodeBindings.stageFor('image-1')).toBe(stageBinding)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1')).toBe(actionBinding)
      expect(harness.runtime.viewModel.nodeBindings.stageFor('image-2')).toBe(inactiveStageBinding)
      expect(inactiveStageBinding.active).toBe(false)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-2')).toBe(
        inactiveActionBinding,
      )
    } finally {
      harness.unmount()
    }
  })

  it('refreshes cached node bindings when relevant state changes', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      const stageBefore = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      harness.runtime.commands.chrome.setMaskBrushSize(48)
      const stageAfter = harness.runtime.viewModel.nodeBindings.stageFor('image-1')

      expect(stageAfter).toBe(stageBefore)
      expect(stageAfter.session.inpaint.brushSize.value).toBe(48)
      expect(harness.runtime.viewModel.nodeBindings.stageFor('image-1')).toBe(stageAfter)

      const actionBefore = harness.runtime.viewModel.nodeBindings.actionFor('image-1')
      harness.activeStrengthAction.value = IMAGE_STRENGTH_ACTIONS[0]
      const actionAfter = harness.runtime.viewModel.nodeBindings.actionFor('image-1')

      expect(actionAfter).not.toBe(actionBefore)
      expect(actionAfter.strengthControl.activeAction).toBe(IMAGE_STRENGTH_ACTIONS[0])
      expect(actionAfter.strengthControl.visible).toBe(true)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1')).toBe(actionAfter)
    } finally {
      harness.unmount()
    }
  })

  it('keeps provider availability separate from per-node generation state', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').directGenerating).toBe(
        false,
      )
      expect(harness.runtime.viewModel.chrome.value.mask.directGenerating).toBe(false)

      const actionBefore = harness.runtime.viewModel.nodeBindings.actionFor('image-1')
      harness.generationAvailable.value = false
      const actionAfter = harness.runtime.viewModel.nodeBindings.actionFor('image-1')

      expect(actionAfter).toBe(actionBefore)
      expect(actionAfter.directGenerating).toBe(false)
      expect(harness.runtime.viewModel.nodeBindings.actionFor('empty-image').directGenerating).toBe(
        false,
      )
      expect(harness.runtime.viewModel.chrome.value.mask.directGenerating).toBe(false)

      harness.directActionRunningNodeIds.value = new Set(['image-1'])
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').directGenerating).toBe(
        true,
      )
      expect(harness.runtime.viewModel.nodeBindings.actionFor('empty-image').directGenerating).toBe(
        false,
      )
      expect(harness.runtime.viewModel.chrome.value.mask.directGenerating).toBe(true)

      harness.directActionRunningNodeIds.value = new Set()

      harness.generationAvailable.value = true
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').directGenerating).toBe(
        false,
      )
      expect(harness.runtime.viewModel.chrome.value.mask.directGenerating).toBe(false)
    } finally {
      harness.unmount()
    }
  })

  it('keeps stage canvas ref callbacks stable and scoped to their node ids', () => {
    const harness = mountCanvasImageTools()

    try {
      const node1StageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const node1StageBindingAgain = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const node2StageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-2')

      expect(node1StageBindingAgain).toBe(node1StageBinding)
      expect(node2StageBinding.nodeId).toBe('image-2')

      expect(harness.runtime.commands.lifecycle.enter('image-2', 'inpaint')).toBe(true)
      const maskCanvas = document.createElement('canvas')
      maskCanvas.width = 4
      maskCanvas.height = 4
      node2StageBinding.session.inpaint.setMaskCanvasRef('image-2', maskCanvas)
      harness.runtime.commands.chrome.submitMaskPrompt('重绘 image-2')

      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-2').promptPlaceholder).toBe(
        'Describe how to repaint the selection',
      )
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').promptPlaceholder).toBe(
        'Optional instructions',
      )
    } finally {
      harness.unmount()
    }
  })

  it('runs inpaint from committed mask data', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      const canvas = document.createElement('canvas')
      canvas.width = 4
      canvas.height = 4
      harness.runtime.viewModel.nodeBindings
        .stageFor('image-1')
        .session.inpaint.setMaskCanvasRef('image-1', canvas)

      harness.runtime.commands.chrome.submitMaskPrompt('重绘这块区域')

      expect(harness.runInpaintAction).toHaveBeenCalledWith('image-1', {
        prompt: '重绘这块区域',
        maskDataUrl: 'data:image/png;base64,execution-mask',
      })
      expect(harness.runtime.viewModel.active.nodeId.value).toBeNull()
    } finally {
      harness.unmount()
    }
  })

  it('binds chrome commands and stage state without leaking workflow commands', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      harness.runtime.commands.chrome.setMaskBrushSize(36)
      harness.runtime.commands.chrome.setMaskLayerOpacity(40)

      const chromeModel = harness.runtime.viewModel.chrome.value
      const stageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const actionBinding = harness.runtime.viewModel.nodeBindings.actionFor('image-1')

      expect(chromeModel.mode).toBe('inpaint')
      expect(chromeModel.mask.brushSize).toBe(36)
      expect(chromeModel.mask.layerOpacity).toBe(40)
      expect(stageBinding.session.inpaint.brushSize.value).toBe(36)
      expect(actionBinding.promptComposerVisible).toBe(true)
      expect('getCommittedMaskData' in harness.runtime).toBe(false)
      expect('clearCommittedMaskData' in harness.runtime).toBe(false)
    } finally {
      harness.unmount()
    }
  })
})
