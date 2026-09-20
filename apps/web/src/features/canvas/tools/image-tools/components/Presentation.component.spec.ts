import { computed, defineComponent, nextTick, shallowRef, type ComputedRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../../test/render'
import type {
  ImageToolChromeCommands,
  ImageToolChromeModel,
  ImageToolPresentationBinding,
} from '../bindings'
import Presentation from './Presentation.vue'

const MaskChromeStub = defineComponent({
  name: 'ImageInpaintChrome',
  props: {
    brushSize: { type: Number, required: true },
    maskLayerOpacity: { type: Number, required: true },
  },
  emits: ['set-brush-size', 'set-mask-layer-opacity'],
  template: `
    <div data-testid="mask-chrome">
      <button data-testid="mask-brush-size" @click="$emit('set-brush-size', 48)">
        Size {{ brushSize }}
      </button>
      <button data-testid="mask-opacity" @click="$emit('set-mask-layer-opacity', 60)">
        Opacity {{ maskLayerOpacity }}
      </button>
    </div>
  `,
})

const EditorChromeStub = defineComponent({
  name: 'Chrome',
  props: { selectedLayerId: { type: String, required: true } },
  emits: ['select-retouch'],
  template: `
    <button data-testid="editor-chrome" @click="$emit('select-retouch', 'hsl')">
      Editor {{ selectedLayerId }}
    </button>
  `,
})

function createChromeCommands(): ImageToolChromeCommands {
  return {
    setMaskDrawMode: vi.fn(),
    setMaskBrushSize: vi.fn(),
    setMaskLayerOpacity: vi.fn(),
    clearMask: vi.fn(),
    undoMask: vi.fn(),
    redoMask: vi.fn(),
    submitMaskPrompt: vi.fn(),
    selectEffectLayer: vi.fn(),
    selectEditorLayer: vi.fn(),
    setEditorLayerVisible: vi.fn(),
    addEditorLayer: vi.fn(),
    copyEditorLayer: vi.fn(),
    mergeEditorLayer: vi.fn(),
    removeEditorLayer: vi.fn(),
    updateEditorParam: vi.fn(),
    updateEditorRetouchParams: vi.fn(),
    applyRetouch: vi.fn(async () => {}),
  }
}

function createChromeModel(overrides: Partial<ImageToolChromeModel> = {}): ImageToolChromeModel {
  return {
    nodeId: 'image-node-1',
    mode: 'inpaint',
    anchorRect: { top: 10, left: 20, width: 300, height: 200 },
    isReady: true,
    mask: {
      drawMode: 'paint',
      brushSize: 24,
      layerOpacity: 40,
      directGenerating: false,
      canUndo: false,
      canRedo: false,
    },
    editor: {
      layers: [],
      selectedLayerId: 'base',
      selectedEffectLayer: null,
      histogramSource: null,
      applyingRetouch: false,
    },
    ...overrides,
  }
}

function renderPresentation(model: ImageToolChromeModel, commands = createChromeCommands()) {
  const activeModel = shallowRef(model)
  return {
    ...render(Presentation, {
      props: {
        imageTools: {
          viewModel: {
            chrome: computed(() => activeModel.value) as ComputedRef<ImageToolChromeModel>,
          },
          commands: { chrome: commands },
        } satisfies ImageToolPresentationBinding,
      },
      global: {
        stubs: { ImageInpaintChrome: MaskChromeStub, Chrome: EditorChromeStub },
      },
    }),
    commands,
    async setModel(nextModel: ImageToolChromeModel) {
      activeModel.value = nextModel
      await nextTick()
    },
  }
}

describe('Presentation', () => {
  it('renders Mask chrome and dispatches its command through the Image Tool session', async () => {
    const view = renderPresentation(createChromeModel())

    expect(screen.getByTestId('mask-brush-size')).toHaveTextContent('Size 24')
    expect(screen.getByTestId('mask-opacity')).toHaveTextContent('Opacity 40')
    await screen.getByTestId('mask-brush-size').click()
    await screen.getByTestId('mask-opacity').click()
    expect(view.commands.setMaskBrushSize).toHaveBeenCalledWith(48)
    expect(view.commands.setMaskLayerOpacity).toHaveBeenCalledWith(60)
  })

  it('renders Editor chrome and dispatches its command through the Image Tool session', async () => {
    const view = renderPresentation(createChromeModel({ mode: 'adjust' }))

    expect(screen.getByTestId('editor-chrome')).toHaveTextContent('Editor base')
    await screen.getByTestId('editor-chrome').click()
    expect(view.commands.selectEffectLayer).toHaveBeenCalledWith('hsl')
  })

  it('renders no chrome when Image Tool is inactive or not ready', () => {
    renderPresentation(createChromeModel({ nodeId: null }))
    expect(screen.queryByTestId('mask-chrome')).toBeNull()
    expect(screen.queryByTestId('editor-chrome')).toBeNull()

    renderPresentation(createChromeModel({ isReady: false }))
    expect(screen.queryByTestId('mask-chrome')).toBeNull()
    expect(screen.queryByTestId('editor-chrome')).toBeNull()
  })

  it('switches chrome mode without retaining the previous tool surface', async () => {
    const view = renderPresentation(createChromeModel())

    expect(screen.getByTestId('mask-chrome')).toBeInTheDocument()
    expect(screen.queryByTestId('editor-chrome')).toBeNull()

    await view.setModel(createChromeModel({ mode: 'adjust' }))

    expect(screen.queryByTestId('mask-chrome')).toBeNull()
    expect(screen.getByTestId('editor-chrome')).toBeInTheDocument()
  })
})
