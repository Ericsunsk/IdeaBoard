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
import { cleanup } from '@testing-library/vue'
import { render } from '../../../../test/render'
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

const MaskOverlayStub = createCanvasOverlayStub('inpaint-overlay')
const RenderedPreviewStub = defineComponent({
  props: {
    preview: { type: Object, required: true },
  },
  setup() {
    return () => h('canvas', { 'data-rendered-preview': '' })
  },
})

function createBinding(
  mode: ImageToolStageBinding['mode'],
  nodeId = 'node-1',
  options: {
    inpaintNodeId?: string
    maskOverlayStyle?: Record<string, string>
    renderedPreview?: ImageToolStageBinding['renderedPreview']
  } = {},
) {
  const setMaskCanvasRef = vi.fn()
  const onCanvasPointerDown = vi.fn()
  const inpaintNodeId = options.inpaintNodeId ?? nodeId

  return {
    binding: {
      nodeId,
      mode,
      renderedPreview: options.renderedPreview ?? null,
      session: {
        inpaint: {
          inpaintNodeId: ref(inpaintNodeId),
          inpaintOverlayStyle: computed(
            () => options.maskOverlayStyle ?? { width: '20.2px', height: '10.6px' },
          ),
          brushSize: ref(20),
          setMaskCanvasRef,
          onCanvasPointerDown,
        },
        maskChromeState: { inpaintLayerOpacity: ref(50) },
      },
    } as unknown as ImageToolStageBinding,
    onCanvasPointerDown,
    setMaskCanvasRef,
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
        ImageInpaintOverlay: MaskOverlayStub,
        RenderedImagePreviewCanvas: RenderedPreviewStub,
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
  it('keeps the media slot neutral without a selected tool and releases the mask ref on unmount', () => {
    const current = createBinding(null)
    const view = renderPresentation(current.binding)

    expect(view.container.querySelector('[data-stage-image]')).toBeTruthy()
    expect(view.container.querySelector('.retouch-raster-overlay')).toBeNull()
    expect(view.container.querySelector('.inpaint-overlay')).toBeNull()

    cleanup()

    expect(current.setMaskCanvasRef).toHaveBeenCalledExactlyOnceWith('node-1', null)
  })

  it('replaces source media with the rendered adjustment preview', () => {
    const source = createBinding('adjust', 'node-1')
    const rendered = createBinding('adjust', 'node-2', {
      renderedPreview: {
        type: 'rendered-image',
        imageData: new ImageData(2, 2),
        width: 2,
        height: 2,
      },
    })
    const sourceView = renderPresentation(source.binding)
    const renderedView = renderPresentation(rendered.binding)

    expect(sourceView.container.querySelector('[data-stage-image]')).toBeTruthy()
    expect(sourceView.container.querySelector('[data-rendered-preview]')).toBeNull()
    expect(renderedView.container.querySelector('[data-stage-image]')).toBeNull()
    expect(renderedView.container.querySelector('[data-rendered-preview]')).toBeTruthy()
  })

  it('renders the inpaint canvas at its derived mask dimensions and forwards its pointer event', () => {
    const current = createBinding('inpaint')
    const view = renderPresentation(current.binding)
    const canvas = view.container.querySelector<HTMLCanvasElement>('.inpaint-overlay')
    const pointerDown = new Event('pointerdown', { bubbles: true }) as PointerEvent

    expect(canvas).toBeInstanceOf(HTMLCanvasElement)
    expect(canvas).toHaveAttribute('data-canvas-width', '20')
    expect(canvas).toHaveAttribute('data-canvas-height', '11')
    expect(canvas?.style.height).toBe('10.6px')
    expect(current.setMaskCanvasRef).toHaveBeenCalledWith('node-1', canvas)

    canvas?.dispatchEvent(pointerDown)

    expect(current.onCanvasPointerDown).toHaveBeenCalledExactlyOnceWith(pointerDown)
  })

  it('releases the previous mask canvas before switching nodes and modes', async () => {
    const firstInpaint = createBinding('inpaint', 'node-1')
    const secondInpaint = createBinding('inpaint', 'node-2')
    const adjust = createBinding('adjust', 'node-2')
    const view = renderPresentation(firstInpaint.binding)

    await view.setBinding(secondInpaint.binding)

    expect(firstInpaint.setMaskCanvasRef).toHaveBeenCalledWith('node-1', null)
    expect(secondInpaint.setMaskCanvasRef).toHaveBeenCalledWith(
      'node-2',
      expect.any(HTMLCanvasElement),
    )

    await view.setBinding(adjust.binding)

    expect(view.container.querySelector('.inpaint-overlay')).toBeNull()
    expect(secondInpaint.setMaskCanvasRef).toHaveBeenCalledWith('node-2', null)
  })
})
