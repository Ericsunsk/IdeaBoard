import type { Connection, Node } from '@vue-flow/core'
import {
  createApp,
  defineComponent,
  h,
  inject,
  nextTick,
  ref,
  shallowRef,
  type Ref,
} from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
} from '../image-editor'
import {
  cancelImageAdjustmentFinalRender,
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  renderImageAdjustmentFile,
  type WorkspaceImageAdjustmentFinalApplyContext,
} from '../image-editor'
import { useWorkspaceImageToolRuntime } from './useWorkspaceImageToolRuntime'

vi.mock('../image-editor', async (importOriginal) => {
  const { computed } = await import('vue')
  const actual = await importOriginal<typeof import('../image-editor')>()
  return {
    ...actual,
    cancelImageAdjustmentFinalRender: vi.fn(),
    renderImageAdjustmentFile: vi.fn(),
    useImageAdjustmentRenderedPreview: vi.fn(() => ({
      imageEditorRenderedPreview: computed(() => null),
    })),
  }
})

const cancelFinalRenderMock = vi.mocked(cancelImageAdjustmentFinalRender)
const renderAdjustmentFileMock = vi.mocked(renderImageAdjustmentFile)

type WorkspaceImageToolRuntime = ReturnType<typeof useWorkspaceImageToolRuntime>

const effectiveInvertLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:invert',
  type: 'adjustment',
  kind: 'invert',
  name: '反相 1',
  visible: true,
  params: { enabled: true },
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

function mountWorkspaceImageToolRuntime() {
  const stage = createStageElement()
  const activeTool = ref('select')
  const currentBoardId = ref('board-1')
  const nodes: Ref<Node[]> = shallowRef(
    Object.keys(imageNodeFixtures).map((id) => ({ id, data: { kind: 'image' } })) as Node[],
  )
  const connectImageNodes = vi.fn<(connection: Connection) => void>()
  const createImageNodeFromFile = vi.fn(async () => 'node-created')
  let imageAdjustmentFinalApplyContext: WorkspaceImageAdjustmentFinalApplyContext | null = null
  const message = {
    warning: vi.fn<(message: string) => void>(),
    success: vi.fn<(message: string) => void>(),
  }
  let runtime!: WorkspaceImageToolRuntime
  const host = document.createElement('div')
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
        runtime = useWorkspaceImageToolRuntime({
          nodes,
          activeTool,
          currentBoardId,
          getImageNodeDisplayPayload: (nodeId) => imageNodeFixtures[nodeId]?.display ?? null,
          getImageNodeSnapshot: (nodeId): ImageNodeItem | null =>
            imageNodeFixtures[nodeId]?.snapshot ?? null,
          getImageStageElement: (nodeId) => (imageNodeFixtures[nodeId] ? stage : null),
          createImageNodeFromFile,
          connectImageNodes,
          clearSelectedFlowElements: vi.fn(),
          startCanvasInteraction: vi.fn(),
          stopCanvasInteraction: vi.fn(),
          message,
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
    cancelImageAdjustmentFinalApply: () => {
      if (!imageAdjustmentFinalApplyContext) {
        throw new Error('Image adjustment final apply context was not provided')
      }
      imageAdjustmentFinalApplyContext.cancel()
    },
    message,
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
  renderAdjustmentFileMock.mockReset()
})

describe('useWorkspaceImageToolRuntime image adjustment final apply', () => {
  it('records final render progress while applying and clears it after success', async () => {
    let resolveRender!: (file: File) => void
    renderAdjustmentFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((resolve) => {
          resolveRender = resolve
          options?.onProgress?.({ phase: 'render', loaded: 24, total: 96 })
        }),
    )
    const harness = mountWorkspaceImageToolRuntime()

    try {
      harness.runtime.enterFlowImageEditorMode('node-1', 'adjust')
      harness.runtime.imageEditorLayers.value = [effectiveInvertLayer, sourceLayer]

      const apply = harness.runtime.onApplyImageEditorAdjustments()
      await flushPromises()

      expect(harness.runtime.isApplyingImageEditorAdjustments.value).toBe(true)
      expect(harness.runtime.imageAdjustmentFinalApplyProgress.value).toEqual({
        phase: 'render',
        loaded: 24,
        total: 96,
      })
      expect(renderAdjustmentFileMock).toHaveBeenCalledWith(
        { url: 'https://example.com/source.png', title: 'Source' },
        [effectiveInvertLayer],
        expect.objectContaining({
          taskId: 'image-adjustment-final:node-1',
          latestOnly: true,
          onProgress: expect.any(Function),
        }),
      )

      resolveRender(new File(['rendered'], 'rendered.webp', { type: 'image/webp' }))
      await apply

      expect(harness.runtime.isApplyingImageEditorAdjustments.value).toBe(false)
      expect(harness.runtime.imageAdjustmentFinalApplyProgress.value).toBeNull()
      expect(harness.message.success).toHaveBeenCalledWith('已应用编辑并派生新节点')
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
    renderAdjustmentFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((_resolve, reject) => {
          options?.onProgress?.({ phase: 'encode' })
          window.setTimeout(() => reject(new Error('cancelled')), 0)
        }),
    )
    const harness = mountWorkspaceImageToolRuntime()

    try {
      harness.runtime.enterFlowImageEditorMode('node-1', 'adjust')
      harness.runtime.imageEditorLayers.value = [effectiveInvertLayer, sourceLayer]

      const apply = harness.runtime.onApplyImageEditorAdjustments()
      await flushPromises()
      harness.cancelImageAdjustmentFinalApply()

      expect(cancelFinalRenderMock).toHaveBeenCalledWith('image-adjustment-final:node-1')
      expect(harness.runtime.isApplyingImageEditorAdjustments.value).toBe(false)
      expect(harness.runtime.imageAdjustmentFinalApplyProgress.value).toBeNull()

      await apply

      expect(harness.message.warning).not.toHaveBeenCalled()
      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('does not write an old final render result back after switching to another image node', async () => {
    let resolveOldRender!: (file: File) => void
    renderAdjustmentFileMock.mockImplementation(
      async (_source, _layers, options): Promise<File> =>
        new Promise((resolve) => {
          resolveOldRender = resolve
          options?.onProgress?.({ phase: 'render', loaded: 1, total: 4 })
        }),
    )
    const harness = mountWorkspaceImageToolRuntime()

    try {
      harness.runtime.enterFlowImageEditorMode('node-1', 'adjust')
      harness.runtime.imageEditorLayers.value = [effectiveInvertLayer, sourceLayer]

      const oldApply = harness.runtime.onApplyImageEditorAdjustments()
      await flushPromises()

      expect(harness.runtime.isApplyingImageEditorAdjustments.value).toBe(true)
      expect(harness.runtime.enterFlowImageEditorMode('node-2', 'adjust')).toBe(true)
      expect(cancelFinalRenderMock).toHaveBeenCalledWith('image-adjustment-final:node-1')
      expect(harness.runtime.flowImageEditorNodeId.value).toBe('node-2')
      expect(harness.runtime.isApplyingImageEditorAdjustments.value).toBe(false)

      resolveOldRender(new File(['old'], 'old.webp', { type: 'image/webp' }))
      await oldApply

      expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
      expect(harness.connectImageNodes).not.toHaveBeenCalled()
      expect(harness.message.success).not.toHaveBeenCalled()
      expect(harness.message.warning).not.toHaveBeenCalled()
      expect(harness.runtime.flowImageEditorNodeId.value).toBe('node-2')
    } finally {
      harness.unmount()
    }
  })
})
