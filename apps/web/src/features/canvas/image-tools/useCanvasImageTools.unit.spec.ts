/* eslint-disable vue/one-component-per-file */
import type { Node } from '@vue-flow/core'
import { createApp, defineComponent, h, inject, nextTick, ref, shallowRef, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import {
  cancelImageAdjustmentFinalRender,
  renderImageEditorLayerStackFile,
} from '../image-editor/services/imageAdjustmentRenderer'
import {
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  type CanvasImageAdjustmentFinalApplyContext,
} from '../image-editor/runtime/imageAdjustmentFinalApplyContext'
import { IMAGE_STRENGTH_ACTIONS, type ImageStrengthAction } from '../execution'
import type { UseCanvasImageToolsOptions } from './imageToolSession'
import { useCanvasImageTools, type CanvasImageTools } from '.'

const rasterOverlayMocks = vi.hoisted(() => ({
  setRasterCanvasRef: vi.fn<(nodeId: string, element: Element | null) => void>(),
  onRasterCanvasPointerDown: vi.fn<(event: PointerEvent) => void>(),
}))

vi.mock('../image-editor/services/imageAdjustmentRenderer', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../image-editor/services/imageAdjustmentRenderer')>()
  return {
    ...actual,
    cancelImageAdjustmentFinalRender: vi.fn(),
    renderImageEditorLayerStackFile: vi.fn(),
  }
})

vi.mock('../image-editor/composables/useImageAdjustmentRenderedPreview', async () => {
  const { computed } = await import('vue')
  return {
    useImageAdjustmentRenderedPreview: vi.fn(() => ({
      imageEditorRenderedPreview: computed(() => null),
      imageEditorPreviewSource: computed(() => null),
    })),
  }
})

vi.mock('../image-editor/composables/useImageEditorRasterOverlay', async () => {
  const { computed } = await import('vue')
  return {
    useImageEditorRasterOverlay: vi.fn(() => ({
      rasterOverlayStyle: computed(() => null),
      rasterCanvasWidth: computed(() => 1),
      rasterCanvasHeight: computed(() => 1),
      rasterBrushSize: computed(() => 32),
      rasterTool: computed(() => null),
      rasterToolEnabled: computed(() => false),
      hasRasterEdits: computed(() => false),
      setRasterCanvasRef: rasterOverlayMocks.setRasterCanvasRef,
      onRasterCanvasPointerDown: rasterOverlayMocks.onRasterCanvasPointerDown,
    })),
  }
})

vi.mock('../mask-editor/lib/maskPaintingCanvas', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../mask-editor/lib/maskPaintingCanvas')>()
  return {
    ...actual,
    buildMaskCommitResult: vi.fn(() => ({
      executionMaskDataUrl: 'data:image/png;base64,execution-mask',
      maskDataUrl: 'data:image/png;base64,preview-mask',
    })),
  }
})

const cancelFinalRenderMock = vi.mocked(cancelImageAdjustmentFinalRender)
const renderLayerStackFileMock = vi.mocked(renderImageEditorLayerStackFile)

const imageNodeFixtures: Record<
  string,
  {
    display: { url: string; ratio: number }
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
  const nodes: Ref<Node[]> = shallowRef(
    Object.keys(imageNodeFixtures).map((id) => ({ id, data: { kind: 'image' } })) as Node[],
  )
  const connectionAdapter = { connectIntent: vi.fn(() => true) }
  const createImageNodeFromFile = vi.fn<UseCanvasImageToolsOptions['createImageNodeFromFile']>(
    async () => 'image-3',
  )
  const setViewport = vi.fn()
  const setCenter = vi.fn()
  const runInpaintAction = vi.fn()
  const runPromptGeneration = vi.fn()
  const updateOutputCount = vi.fn()
  const activeStrengthAction = ref<ImageStrengthAction | null>(null)
  let imageAdjustmentFinalApplyContext: CanvasImageAdjustmentFinalApplyContext | null = null
  const feedback = {
    warning: vi.fn<(message: string) => void>(),
    succeeded: vi.fn<(message: string) => void>(),
    actionUnavailable: vi.fn<(message: string) => void>(),
  }
  let runtime!: CanvasImageTools
  const host = document.createElement('div')
  canvasWrapRef.value = host
  document.body.appendChild(host)
  const ContextProbe = defineComponent({
    setup() {
      imageAdjustmentFinalApplyContext = inject(IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY, null)
      return () => null
    },
  })
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
          createImageNodeFromFile,
          connectionAdapter,
          uploadImageToBoard: async () => ({ url: 'https://example.com/ref.png', ratio: 1 }),
          openImageUrlPreview: vi.fn(),
          clearActiveNodePanel: vi.fn(),
          hideStrengthControl: vi.fn(),
          updateOutputCount,
          runInpaintAction,
          runPromptGeneration,
          isImageActionPanelVisible: (nodeId) => nodeId === 'image-1',
          isImagePromptComposerVisible: (nodeId) => nodeId === 'image-1',
          isGenerationAvailable: () => generationAvailable.value,
          isDirectActionRunning: () => false,
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
        return () => h(ContextProbe)
      },
    }),
  )
  app.mount(host)

  return {
    activeTool,
    activeStrengthAction,
    app,
    connectionAdapter,
    createImageNodeFromFile,
    currentBoardId,
    generationAvailable,
    host,
    isCanvasInteractionLocked,
    runInpaintAction,
    runPromptGeneration,
    updateOutputCount,
    cancelImageAdjustmentFinalApply: () => {
      if (!imageAdjustmentFinalApplyContext) {
        throw new Error('Image adjustment final apply context was not provided')
      }
      imageAdjustmentFinalApplyContext.cancel()
    },
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
  return runtime.viewModel.chrome.value.editor.layers
}

async function flushPromises() {
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  vi.restoreAllMocks()
  cancelFinalRenderMock.mockReset()
  renderLayerStackFileMock.mockReset()
  rasterOverlayMocks.setRasterCanvasRef.mockReset()
  rasterOverlayMocks.onRasterCanvasPointerDown.mockReset()
})

describe('useCanvasImageTools image adjustment final apply', () => {
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
      expect(harness.runtime.viewModel.adjustment.finalApplyProgress.value).toBeNull()
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
      expect(harness.runtime.commands.prompt.runPromptAction).toEqual(expect.any(Function))
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

  it('records final render progress while applying and clears it after success', async () => {
    let resolveRender!: (file: File) => void
    renderLayerStackFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((resolve) => {
          resolveRender = resolve
          options?.onProgress?.({ phase: 'render', loaded: 24, total: 96 })
        }),
    )
    const harness = mountCanvasImageTools()

    try {
      const renderableLayers = prepareRenderableAdjustmentSession(harness.runtime)

      const apply = harness.runtime.commands.editor.apply()
      await flushPromises()

      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(true)
      expect(harness.runtime.viewModel.adjustment.finalApplyProgress.value).toEqual({
        phase: 'render',
        loaded: 24,
        total: 96,
      })
      expect(renderLayerStackFileMock).toHaveBeenCalledWith(
        { url: 'https://example.com/source.png', title: 'Source' },
        renderableLayers,
        expect.objectContaining({
          scopeId: 'image-1',
          onProgress: expect.any(Function),
        }),
      )

      resolveRender(new File(['rendered'], 'rendered.webp', { type: 'image/webp' }))
      await apply

      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
      expect(harness.runtime.viewModel.adjustment.finalApplyProgress.value).toBeNull()
      expect(harness.feedback.warning).not.toHaveBeenCalled()
      expect(harness.feedback.succeeded).toHaveBeenCalledWith('已应用编辑并派生新节点')
      expect(harness.connectionAdapter.connectIntent).toHaveBeenCalledWith({
        sourceNodeId: 'image-1',
        sourcePort: 'image',
        targetNodeId: 'image-3',
        targetPort: 'image',
      })
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
      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
      expect(harness.connectionAdapter.connectIntent).not.toHaveBeenCalled()
      expect(harness.feedback.warning).toHaveBeenCalledWith('图片节点未能写入画布')
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('keeps the rendered node and editor open when automatic connection is rejected', async () => {
    renderLayerStackFileMock.mockResolvedValue(
      new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    )
    const harness = mountCanvasImageTools()
    harness.connectionAdapter.connectIntent.mockReturnValue(false)

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      await harness.runtime.commands.editor.apply()

      expect(harness.createImageNodeFromFile).toHaveBeenCalledTimes(1)
      expect(harness.connectionAdapter.connectIntent).toHaveBeenCalledTimes(1)
      expect(harness.feedback.warning).toHaveBeenCalledWith(
        '图片已创建，但自动连线失败，请手动连接',
      )
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.runtime.viewModel.active.nodeId.value).toBe('image-1')
      expect(harness.runtime.viewModel.active.mode.value).toBe('adjust')
    } finally {
      harness.unmount()
    }
  })

  it('cancels the active final render without showing a warning toast', async () => {
    renderLayerStackFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((_resolve, reject) => {
          options?.onProgress?.({ phase: 'encode' })
          window.setTimeout(() => reject(new Error('cancelled')), 0)
        }),
    )
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      const apply = harness.runtime.commands.editor.apply()
      await flushPromises()
      harness.cancelImageAdjustmentFinalApply()

      expect(cancelFinalRenderMock).toHaveBeenCalledWith('image-1')
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)
      expect(harness.runtime.viewModel.adjustment.finalApplyProgress.value).toBeNull()

      await apply

      expect(harness.feedback.warning).not.toHaveBeenCalled()
      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('does not write an old final render result back after switching to another image node', async () => {
    let resolveOldRender!: (file: File) => void
    renderLayerStackFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((resolve) => {
          resolveOldRender = resolve
          options?.onProgress?.({ phase: 'render', loaded: 1, total: 4 })
        }),
    )
    const harness = mountCanvasImageTools()

    try {
      prepareRenderableAdjustmentSession(harness.runtime)

      const oldApply = harness.runtime.commands.editor.apply()
      await flushPromises()

      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(true)
      expect(harness.runtime.commands.lifecycle.enter('image-2', 'adjust')).toBe(true)
      expect(cancelFinalRenderMock).toHaveBeenCalledWith('image-1')
      expect(harness.runtime.viewModel.active.nodeId.value).toBe('image-2')
      expect(harness.runtime.viewModel.adjustment.isApplying.value).toBe(false)

      resolveOldRender(new File(['old'], 'old.webp', { type: 'image/webp' }))
      await oldApply

      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
      expect(harness.connectionAdapter.connectIntent).not.toHaveBeenCalled()
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.feedback.warning).not.toHaveBeenCalled()
      expect(harness.runtime.viewModel.active.nodeId.value).toBe('image-2')
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

      expect(stageBinding.mode).toBe('inpaint')
      expect(stageBinding.maskOverlayStyle).toEqual({
        left: '40px',
        top: '0px',
        width: '240px',
        height: '240px',
      })
      expect(stageBinding.maskCanvasWidth).toBe(240)
      expect(stageBinding.maskCanvasHeight).toBe(240)
      expect(actionBinding.actionMenuVisible).toBe(true)
      expect(actionBinding.promptPlaceholder).toBe('输入提示词改造这张图')
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
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-2')).toBe(inactiveActionBinding)
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

      expect(stageAfter).not.toBe(stageBefore)
      expect(stageAfter.maskBrushSize).toBe(48)
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

  it('disables generation controls while the generation provider is unavailable', () => {
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

      expect(actionAfter).not.toBe(actionBefore)
      expect(actionAfter.directGenerating).toBe(true)
      expect(harness.runtime.viewModel.chrome.value.mask.directGenerating).toBe(true)

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

      expect(node1StageBindingAgain.setMaskCanvasRef).toBe(node1StageBinding.setMaskCanvasRef)
      expect(node1StageBindingAgain.setRasterCanvasRef).toBe(
        node1StageBinding.setRasterCanvasRef,
      )
      expect(node2StageBinding.setMaskCanvasRef).not.toBe(node1StageBinding.setMaskCanvasRef)
      expect(node2StageBinding.setRasterCanvasRef).not.toBe(
        node1StageBinding.setRasterCanvasRef,
      )

      const rasterCanvas = document.createElement('canvas')
      node2StageBinding.setRasterCanvasRef(rasterCanvas)
      expect(rasterOverlayMocks.setRasterCanvasRef).toHaveBeenCalledWith('image-2', rasterCanvas)

      expect(harness.runtime.commands.lifecycle.enter('image-2', 'inpaint')).toBe(true)
      const maskCanvas = document.createElement('canvas')
      maskCanvas.width = 4
      maskCanvas.height = 4
      node2StageBinding.setMaskCanvasRef(maskCanvas)
      harness.runtime.commands.chrome.submitMaskPrompt('重绘 image-2', 1)

      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-2').promptPlaceholder).toBe(
        '描述如何重绘选区',
      )
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').promptPlaceholder).toBe(
        '输入提示词改造这张图',
      )
    } finally {
      harness.unmount()
    }
  })

  it('keeps committed mask data inside image-tools when running prompt actions', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      const canvas = document.createElement('canvas')
      canvas.width = 4
      canvas.height = 4
      harness.runtime.viewModel.nodeBindings.stageFor('image-1').setMaskCanvasRef(canvas)

      harness.runtime.commands.chrome.submitMaskPrompt('重绘这块区域', 2)

      expect(harness.runInpaintAction).toHaveBeenCalledWith('image-1', {
        prompt: '重绘这块区域',
        outputCount: 2,
        maskDataUrl: 'data:image/png;base64,execution-mask',
        referenceImageUrls: [],
      })
      expect(harness.runtime.viewModel.active.nodeId.value).toBeNull()
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').promptPlaceholder).toBe(
        '描述如何重绘选区',
      )

      harness.runtime.commands.prompt.runPromptAction('image-1', '继续用选区生成', 3)

      expect(harness.runInpaintAction).toHaveBeenLastCalledWith('image-1', {
        prompt: '继续用选区生成',
        outputCount: 3,
        maskDataUrl: 'data:image/png;base64,execution-mask',
      })
      expect(harness.runPromptGeneration).not.toHaveBeenCalled()

      harness.runtime.commands.lifecycle.discardNodeEdits('image-1')
      expect(harness.runtime.viewModel.nodeBindings.actionFor('image-1').promptPlaceholder).toBe(
        '输入提示词改造这张图',
      )

      harness.runtime.commands.prompt.runPromptAction('image-1', '普通生成', 1)
      expect(harness.runPromptGeneration).toHaveBeenCalledWith('image-1', '普通生成', 1)
    } finally {
      harness.unmount()
    }
  })

  it('binds chrome commands and stage state without leaking workflow commands', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.commands.lifecycle.enter('image-1', 'inpaint')).toBe(true)

      harness.runtime.commands.chrome.setMaskBrushShape('square')
      harness.runtime.commands.chrome.setMaskBrushSize(36)
      harness.runtime.commands.chrome.setMaskLayerOpacity(40)
      harness.runtime.commands.chrome.updateMaskOutputCount(3)

      const chromeModel = harness.runtime.viewModel.chrome.value
      const stageBinding = harness.runtime.viewModel.nodeBindings.stageFor('image-1')
      const actionBinding = harness.runtime.viewModel.nodeBindings.actionFor('image-1')

      expect(chromeModel.mode).toBe('inpaint')
      expect(chromeModel.mask.brushShape).toBe('square')
      expect(chromeModel.mask.brushSize).toBe(36)
      expect(chromeModel.mask.layerOpacity).toBe(40)
      expect(stageBinding.maskBrushShape).toBe('square')
      expect(stageBinding.maskBrushSize).toBe(36)
      expect(actionBinding.promptComposerVisible).toBe(true)
      expect(harness.updateOutputCount).toHaveBeenCalledWith('image-1', {
        key: 'outputCount',
        value: 3,
      })
      expect('getCommittedMaskData' in harness.runtime).toBe(false)
      expect('clearCommittedMaskData' in harness.runtime).toBe(false)
    } finally {
      harness.unmount()
    }
  })
})
