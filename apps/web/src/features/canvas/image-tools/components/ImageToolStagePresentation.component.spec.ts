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

vi.mock('../../retouch/utils/retouchWebglEngine', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../retouch/utils/retouchWebglEngine')>()
  return {
    ...actual,
    isWebGLRetouchSupported: () => true,
  }
})

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
function createBinding(
  mode: ImageToolStageBinding['mode'],
  nodeId = 'node-1',
  options: {
    inpaintNodeId?: string
    maskOverlayStyle?: Record<string, string>
    retouchStage?: ImageToolStageBinding['retouchStage']
  } = {},
) {
  const setMaskCanvasRef = vi.fn()
  const setRetouchCanvasRef = vi.fn()
  const onCanvasPointerDown = vi.fn()
  const inpaintNodeId = options.inpaintNodeId ?? nodeId

  const retouchStage =
    options.retouchStage !== undefined
      ? options.retouchStage
      : mode === 'adjust'
        ? {
            canvasRef: ref<HTMLCanvasElement | null>(null),
            setCanvasRef: setRetouchCanvasRef,
            isGpuActive: computed(() => true),
            stageStatus: computed(() => 'ready' as const),
          }
        : null

  return {
    binding: {
      nodeId,
      active: mode != null,
      mode,
      retouchStage,
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
    setRetouchCanvasRef,
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

  it('mounts zero-copy WebGL canvas in adjust mode and binds the canvas ref', () => {
    const source = createBinding(null, 'node-1')
    const adjust = createBinding('adjust', 'node-2')
    const sourceView = renderPresentation(source.binding)
    const adjustView = renderPresentation(adjust.binding)

    expect(sourceView.container.querySelector('[data-stage-image]')).toBeTruthy()
    expect(sourceView.container.querySelector('.image-tool-stage-zero-copy')).toBeNull()
    expect(adjustView.container.querySelector('[data-stage-image]')).toBeTruthy()
    expect(adjustView.container.querySelector('.image-tool-stage-zero-copy')).toBeTruthy()
    expect(adjust.setRetouchCanvasRef).toHaveBeenCalledWith(expect.any(HTMLCanvasElement))
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
