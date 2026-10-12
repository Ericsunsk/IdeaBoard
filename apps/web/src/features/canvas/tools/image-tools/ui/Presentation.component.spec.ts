import { computed, defineComponent, nextTick, shallowRef, type ComputedRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../../test/render'
import type {
  ImageToolChromeCommands,
  ImageToolChromeModel,
  ImageToolPresentationBinding,
} from '../session/bindings'
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
  name: 'RetouchChrome',
  props: {
    selectedLayerId: { type: String, required: true },
    canApply: { type: Boolean, required: true },
    applyingRetouch: { type: Boolean, required: true },
  },
  emits: ['select-retouch', 'apply-retouch'],
  template: `
    <div>
      <button data-testid="editor-chrome" @click="$emit('select-retouch', 'hsl')">
        Editor {{ selectedLayerId }}
      </button>
      <button
        data-testid="editor-apply"
        :disabled="!canApply || applyingRetouch"
        @click="$emit('apply-retouch')"
      >
        Apply {{ canApply ? 'ready' : 'empty' }}
      </button>
    </div>
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
      canApply: true,
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
        // VTU resolves the stub by the SFC filename (Chrome.vue → Chrome),
        // not the barrel alias (RetouchChrome): keep both keys.
        stubs: {
          ImageInpaintChrome: MaskChromeStub,
          RetouchChrome: EditorChromeStub,
          Chrome: EditorChromeStub,
        },
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

  it('injects parent-computed canApply into Editor chrome (same gate as readApplySource)', async () => {
    const view = renderPresentation(
      createChromeModel({
        mode: 'adjust',
        editor: {
          layers: [],
          selectedLayerId: 'base',
          selectedEffectLayer: null,
          histogramSource: null,
          applyingRetouch: false,
          canApply: false,
        },
      }),
    )

    expect(screen.getByTestId('editor-apply')).toBeDisabled()
    expect(screen.getByTestId('editor-apply')).toHaveTextContent('Apply empty')

    await view.setModel(
      createChromeModel({
        mode: 'adjust',
        editor: {
          layers: [],
          selectedLayerId: 'base',
          selectedEffectLayer: null,
          histogramSource: null,
          applyingRetouch: false,
          canApply: true,
        },
      }),
    )

    expect(screen.getByTestId('editor-apply')).not.toBeDisabled()
    await screen.getByTestId('editor-apply').click()
    expect(view.commands.applyRetouch).toHaveBeenCalledTimes(1)

    await view.setModel(
      createChromeModel({
        mode: 'adjust',
        editor: {
          layers: [],
          selectedLayerId: 'base',
          selectedEffectLayer: null,
          histogramSource: null,
          applyingRetouch: true,
          canApply: true,
        },
      }),
    )

    expect(screen.getByTestId('editor-apply')).toBeDisabled()
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
