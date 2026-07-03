/* eslint-disable vue/one-component-per-file */
import type { Connection, Node } from '@vue-flow/core'
import { createApp, defineComponent, h, inject, nextTick, ref, shallowRef, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { ImageEditorAdjustmentLayer, ImageEditorLayer } from '../image-editor'
import {
  cancelImageAdjustmentFinalRender,
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  renderImageEditorLayerStackFile,
  type CanvasImageAdjustmentFinalApplyContext,
} from '../image-editor'
import { useCanvasImageTools, type CanvasImageTools } from '.'

vi.mock('../image-editor', async (importOriginal) => {
  const { computed } = await import('vue')
  const actual = await importOriginal<typeof import('../image-editor')>()
  return {
    ...actual,
    cancelImageAdjustmentFinalRender: vi.fn(),
    renderImageEditorLayerStackFile: vi.fn(),
    useImageAdjustmentRenderedPreview: vi.fn(() => ({
      imageEditorRenderedPreview: computed(() => null),
      imageEditorPreviewSource: computed(() => null),
    })),
    useImageEditorRasterOverlay: vi.fn(() => ({
      rasterOverlayStyle: computed(() => null),
      rasterCanvasWidth: computed(() => 1),
      rasterCanvasHeight: computed(() => 1),
      rasterBrushSize: computed(() => 32),
      rasterTool: computed(() => null),
      rasterToolEnabled: computed(() => false),
      hasRasterEdits: computed(() => false),
      setRasterCanvasRef: vi.fn(),
      onRasterCanvasPointerDown: vi.fn(),
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

const effectiveThresholdLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:threshold',
  type: 'adjustment',
  kind: 'threshold',
  name: '阈值 1',
  visible: true,
  params: { threshold: 128, invert: false },
}

const sourceLayer: ImageEditorLayer = {
  id: 'source',
  type: 'source',
  name: '原图',
  visible: true,
}

const imageNodeFixtures: Record<
  string,
  {
    display: { url: string; ratio: number }
    snapshot: ImageNodeItem
  }
> = {
  'node-1': {
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
  'node-2': {
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
  const nodes: Ref<Node[]> = shallowRef(
    Object.keys(imageNodeFixtures).map((id) => ({ id, data: { kind: 'image' } })) as Node[],
  )
  const connectImageNodes = vi.fn<(connection: Connection) => void>()
  const createImageNodeFromFile = vi.fn(async () => 'node-created')
  const setViewport = vi.fn()
  const setCenter = vi.fn()
  const runInpaintAction = vi.fn()
  const runPromptGeneration = vi.fn()
  const updateOutputCount = vi.fn()
  const activeStrengthAction = ref(null)
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
          connectImageNodes,
          uploadImageToBoard: async () => ({ url: 'https://example.com/ref.png', ratio: 1 }),
          openImageUrlPreview: vi.fn(),
          clearActiveNodePanel: vi.fn(),
          hideStrengthControl: vi.fn(),
          updateOutputCount,
          runInpaintAction,
          runPromptGeneration,
          isImageActionPanelVisible: (nodeId) => nodeId === 'node-1',
          isImagePromptComposerVisible: (nodeId) => nodeId === 'node-1',
          isDirectActionRunning: () => false,
          activeStrengthAction,
          getActionStrength: () => 50,
          setStrengthRangeRef: vi.fn(),
          setStrengthTriggerRef: vi.fn(),
          onStrengthRangePointerDown: vi.fn(),
          onStrengthInput: vi.fn(),
          onStrengthChange: vi.fn(),
          onStrengthPointerUp: vi.fn(),
          onStrengthClick: vi.fn(),
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
    app,
    connectImageNodes,
    createImageNodeFromFile,
    currentBoardId,
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

async function flushPromises() {
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  vi.restoreAllMocks()
  cancelFinalRenderMock.mockReset()
  renderLayerStackFileMock.mockReset()
})

describe('useCanvasImageTools image adjustment final apply', () => {
  it('locks canvas only when a lock-level interaction or editor session is active', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.isCanvasLocked.value).toBe(false)

      harness.isCanvasInteractionLocked.value = true
      expect(harness.runtime.isCanvasLocked.value).toBe(true)

      harness.isCanvasInteractionLocked.value = false
      expect(harness.runtime.isCanvasLocked.value).toBe(false)

      expect(harness.runtime.enter('node-1', 'adjust')).toBe(true)

      expect(harness.runtime.isCanvasLocked.value).toBe(true)
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
      harness.runtime.enter('node-1', 'adjust')
      harness.runtime.editorLayers.value = [effectiveThresholdLayer, sourceLayer]

      const apply = harness.runtime.apply()
      await flushPromises()

      expect(harness.runtime.isApplyingAdjustments.value).toBe(true)
      expect(harness.runtime.adjustmentFinalApplyProgress.value).toEqual({
        phase: 'render',
        loaded: 24,
        total: 96,
      })
      expect(renderLayerStackFileMock).toHaveBeenCalledWith(
        { url: 'https://example.com/source.png', title: 'Source' },
        [effectiveThresholdLayer, sourceLayer],
        expect.objectContaining({
          scopeId: 'node-1',
          onProgress: expect.any(Function),
        }),
      )

      resolveRender(new File(['rendered'], 'rendered.webp', { type: 'image/webp' }))
      await apply

      expect(harness.runtime.isApplyingAdjustments.value).toBe(false)
      expect(harness.runtime.adjustmentFinalApplyProgress.value).toBeNull()
      expect(harness.feedback.succeeded).toHaveBeenCalledWith('已应用编辑并派生新节点')
      expect(harness.connectImageNodes).toHaveBeenCalledWith({
        source: 'node-1',
        sourceHandle: 'out:image',
        target: 'node-created',
        targetHandle: 'in:image',
      })
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
      harness.runtime.enter('node-1', 'adjust')
      harness.runtime.editorLayers.value = [effectiveThresholdLayer, sourceLayer]

      const apply = harness.runtime.apply()
      await flushPromises()
      harness.cancelImageAdjustmentFinalApply()

      expect(cancelFinalRenderMock).toHaveBeenCalledWith('node-1')
      expect(harness.runtime.isApplyingAdjustments.value).toBe(false)
      expect(harness.runtime.adjustmentFinalApplyProgress.value).toBeNull()

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
      harness.runtime.enter('node-1', 'adjust')
      harness.runtime.editorLayers.value = [effectiveThresholdLayer, sourceLayer]

      const oldApply = harness.runtime.apply()
      await flushPromises()

      expect(harness.runtime.isApplyingAdjustments.value).toBe(true)
      expect(harness.runtime.enter('node-2', 'adjust')).toBe(true)
      expect(cancelFinalRenderMock).toHaveBeenCalledWith('node-1')
      expect(harness.runtime.activeNodeId.value).toBe('node-2')
      expect(harness.runtime.isApplyingAdjustments.value).toBe(false)

      resolveOldRender(new File(['old'], 'old.webp', { type: 'image/webp' }))
      await oldApply

      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
      expect(harness.connectImageNodes).not.toHaveBeenCalled()
      expect(harness.feedback.succeeded).not.toHaveBeenCalled()
      expect(harness.feedback.warning).not.toHaveBeenCalled()
      expect(harness.runtime.activeNodeId.value).toBe('node-2')
    } finally {
      harness.unmount()
    }
  })

  it('exposes typed stage and action bindings for the active image node', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.enter('node-1', 'inpaint')).toBe(true)

      const stageBinding = harness.runtime.stageBindingFor('node-1')
      const actionBinding = harness.runtime.actionBindingFor('node-1')

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

  it('keeps committed mask data inside image-tools when running prompt actions', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.enter('node-1', 'inpaint')).toBe(true)

      const canvas = document.createElement('canvas')
      canvas.width = 4
      canvas.height = 4
      harness.runtime.stageBindingFor('node-1').setMaskCanvasRef(canvas)

      harness.runtime.chromeCommands.submitMaskPrompt('重绘这块区域', 2)

      expect(harness.runInpaintAction).toHaveBeenCalledWith('node-1', {
        prompt: '重绘这块区域',
        outputCount: 2,
        maskDataUrl: 'data:image/png;base64,execution-mask',
        referenceImageUrls: [],
      })
      expect(harness.runtime.activeNodeId.value).toBeNull()
      expect(harness.runtime.actionBindingFor('node-1').promptPlaceholder).toBe('描述如何重绘选区')

      harness.runtime.runPromptAction('node-1', '继续用选区生成', 3)

      expect(harness.runInpaintAction).toHaveBeenLastCalledWith('node-1', {
        prompt: '继续用选区生成',
        outputCount: 3,
        maskDataUrl: 'data:image/png;base64,execution-mask',
      })
      expect(harness.runPromptGeneration).not.toHaveBeenCalled()

      harness.runtime.discardNodeEdits('node-1')
      expect(harness.runtime.actionBindingFor('node-1').promptPlaceholder).toBe(
        '输入提示词改造这张图',
      )

      harness.runtime.runPromptAction('node-1', '普通生成', 1)
      expect(harness.runPromptGeneration).toHaveBeenCalledWith('node-1', '普通生成', 1)
    } finally {
      harness.unmount()
    }
  })

  it('binds chrome commands and stage state without leaking workflow commands', () => {
    const harness = mountCanvasImageTools()

    try {
      expect(harness.runtime.enter('node-1', 'inpaint')).toBe(true)

      harness.runtime.chromeCommands.setMaskBrushShape('square')
      harness.runtime.chromeCommands.setMaskBrushSize(36)
      harness.runtime.chromeCommands.setMaskLayerOpacity(40)
      harness.runtime.chromeCommands.updateMaskOutputCount(3)

      const chromeModel = harness.runtime.chromeModel.value
      const stageBinding = harness.runtime.stageBindingFor('node-1')
      const actionBinding = harness.runtime.actionBindingFor('node-1')

      expect(chromeModel.mode).toBe('inpaint')
      expect(chromeModel.mask.brushShape).toBe('square')
      expect(chromeModel.mask.brushSize).toBe(36)
      expect(chromeModel.mask.layerOpacity).toBe(40)
      expect(stageBinding.maskBrushShape).toBe('square')
      expect(stageBinding.maskBrushSize).toBe(36)
      expect(actionBinding.promptComposerVisible).toBe(true)
      expect(harness.updateOutputCount).toHaveBeenCalledWith('node-1', {
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
