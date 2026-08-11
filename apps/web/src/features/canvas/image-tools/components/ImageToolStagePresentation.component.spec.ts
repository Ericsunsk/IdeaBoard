/* eslint-disable vue/one-component-per-file -- Behavior stubs keep presentation tests at the props and DOM seam. */
import {
  computed,
  defineComponent,
  h,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  type PropType,
} from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '../../../../test/render'
import type { ImageToolStageBinding } from '../imageToolBindings'
import ImageToolStagePresentation from './ImageToolStagePresentation.vue'

type CanvasRefSetter = (element: Element | null) => void

function createCanvasOverlayStub(className: string) {
  return defineComponent({
    props: {
      canvasHeight: { type: Number, required: true },
      canvasWidth: { type: Number, required: true },
      setCanvasRef: { type: Function as PropType<CanvasRefSetter>, required: true },
      style: { type: Object as PropType<Record<string, string>>, required: true },
    },
    emits: ['canvas-pointerdown'],
    setup(props, { emit }) {
      const canvasRef = ref<HTMLCanvasElement | null>(null)
      const setCanvasRef = props.setCanvasRef

      onMounted(() => setCanvasRef(canvasRef.value))
      onBeforeUnmount(() => setCanvasRef(null))

      return () =>
        h('canvas', {
          ref: canvasRef,
          class: className,
          'data-canvas-height': props.canvasHeight,
          'data-canvas-width': props.canvasWidth,
          style: props.style,
          onPointerdown: (event: PointerEvent) => emit('canvas-pointerdown', event),
        })
    },
  })
}

const RasterOverlayStub = createCanvasOverlayStub('image-editor-raster-overlay')
const MaskOverlayStub = createCanvasOverlayStub('image-mask-editor-overlay')

function createBinding(
  mode: ImageToolStageBinding['mode'],
  nodeId = 'node-1',
  options: {
    hasRasterEdits?: boolean
    maskEditorNodeId?: string
    maskOverlayStyle?: Record<string, string>
    rasterTool?: string | null
  } = {},
) {
  const setMaskCanvasRef = vi.fn()
  const setRasterCanvasRef = vi.fn()
  const onCanvasPointerDown = vi.fn()
  const onRasterCanvasPointerDown = vi.fn()
  const maskEditorNodeId = options.maskEditorNodeId ?? nodeId

  return {
    binding: {
      nodeId,
      mode,
      renderedPreview: null,
      session: {
        maskEditor: {
          maskEditorNodeId: ref(maskEditorNodeId),
          maskEditorOverlayStyle: computed(
            () => options.maskOverlayStyle ?? { width: '20.2px', height: '10.6px' },
          ),
          brushSize: ref(20),
          brushShape: ref('round'),
          setMaskCanvasRef,
          onCanvasPointerDown,
        },
        maskChromeState: { maskEditorLayerOpacity: ref(50) },
        imageEditorRasterOverlay: {
          rasterOverlayStyle: computed(() => ({ width: '20px', height: '10px' })),
          rasterCanvasWidth: ref(20),
          rasterCanvasHeight: ref(10),
          rasterBrushSize: ref(20),
          rasterToolEnabled: ref(true),
          hasRasterEdits: ref(options.hasRasterEdits ?? true),
          rasterTool: ref(options.rasterTool ?? 'brush'),
          setRasterCanvasRef,
          onRasterCanvasPointerDown,
        },
      },
    } as unknown as ImageToolStageBinding,
    onCanvasPointerDown,
    onRasterCanvasPointerDown,
    setMaskCanvasRef,
    setRasterCanvasRef,
  }
}

function renderPresentation(binding: ImageToolStageBinding) {
  const activeBinding = shallowRef(binding)
  const Harness = defineComponent({
    setup() {
      return () =>
        h(
          ImageToolStagePresentation,
          { binding: activeBinding.value },
          {
            default: () =>
              h('img', { 'data-stage-image': '', src: '/image.png', alt: '图片节点预览' }),
          },
        )
    },
  })
  const view = render(Harness, {
    global: {
      stubs: {
        ImageEditorRasterOverlay: RasterOverlayStub,
        ImageMaskEditorOverlay: MaskOverlayStub,
        RenderedImagePreviewCanvas: true,
      },
    },
  })

  return {
    ...view,
    async setBinding(nextBinding: ImageToolStageBinding) {
      activeBinding.value = nextBinding
      await nextTick()
    },
  }
}

describe('ImageToolStagePresentation', () => {
  it('keeps the media slot neutral without a selected tool and releases both refs on unmount', () => {
    const current = createBinding(null)
    const view = renderPresentation(current.binding)

    expect(view.container.querySelector('[data-stage-image]')).toBeTruthy()
    expect(view.container.querySelector('.image-editor-raster-overlay')).toBeNull()
    expect(view.container.querySelector('.image-mask-editor-overlay')).toBeNull()

    view.unmount()

    expect(current.setMaskCanvasRef).toHaveBeenCalledExactlyOnceWith('node-1', null)
    expect(current.setRasterCanvasRef).toHaveBeenCalledExactlyOnceWith('node-1', null)
  })

  it('renders the adjustment canvas from presentation props and forwards its pointer event', () => {
    const current = createBinding('adjust')
    const view = renderPresentation(current.binding)
    const canvas = view.container.querySelector<HTMLCanvasElement>('.image-editor-raster-overlay')
    const pointerDown = new Event('pointerdown', { bubbles: true }) as PointerEvent

    expect(canvas).toBeInstanceOf(HTMLCanvasElement)
    expect(canvas).toHaveAttribute('data-canvas-width', '20')
    expect(canvas).toHaveAttribute('data-canvas-height', '10')
    expect(canvas?.style.width).toBe('20px')
    expect(current.setRasterCanvasRef).toHaveBeenCalledWith('node-1', canvas)

    canvas?.dispatchEvent(pointerDown)

    expect(current.onRasterCanvasPointerDown).toHaveBeenCalledExactlyOnceWith(pointerDown)
  })

  it('renders the inpaint canvas at its derived mask dimensions and forwards its pointer event', () => {
    const current = createBinding('inpaint')
    const view = renderPresentation(current.binding)
    const canvas = view.container.querySelector<HTMLCanvasElement>('.image-mask-editor-overlay')
    const pointerDown = new Event('pointerdown', { bubbles: true }) as PointerEvent

    expect(canvas).toBeInstanceOf(HTMLCanvasElement)
    expect(canvas).toHaveAttribute('data-canvas-width', '20')
    expect(canvas).toHaveAttribute('data-canvas-height', '11')
    expect(canvas?.style.height).toBe('10.6px')
    expect(current.setMaskCanvasRef).toHaveBeenCalledWith('node-1', canvas)

    canvas?.dispatchEvent(pointerDown)

    expect(current.onCanvasPointerDown).toHaveBeenCalledExactlyOnceWith(pointerDown)
  })

  it('releases the previous canvas before switching nodes and modes', async () => {
    const firstAdjust = createBinding('adjust', 'node-1')
    const secondAdjust = createBinding('adjust', 'node-2')
    const inpaint = createBinding('inpaint', 'node-2')
    const view = renderPresentation(firstAdjust.binding)

    await view.setBinding(secondAdjust.binding)

    expect(firstAdjust.setRasterCanvasRef).toHaveBeenCalledWith('node-1', null)
    expect(secondAdjust.setRasterCanvasRef).toHaveBeenCalledWith(
      'node-2',
      expect.any(HTMLCanvasElement),
    )

    await view.setBinding(inpaint.binding)

    await waitFor(() => {
      expect(view.container.querySelector('.image-editor-raster-overlay')).toBeNull()
      expect(view.container.querySelector('.image-mask-editor-overlay')).toBeInstanceOf(
        HTMLCanvasElement,
      )
    })

    const maskCanvas = view.container.querySelector<HTMLCanvasElement>('.image-mask-editor-overlay')
    expect(secondAdjust.setRasterCanvasRef).toHaveBeenCalledWith('node-2', null)
    expect(inpaint.setMaskCanvasRef).toHaveBeenCalledWith('node-2', maskCanvas)
  })
})
