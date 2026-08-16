/* eslint-disable vue/one-component-per-file -- Local chrome stubs keep the presentation contract test focused. */
import { computed, defineComponent, nextTick, shallowRef, type ComputedRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../test/render'
import type {
  ImageToolChromeCommands,
  ImageToolChromeModel,
  ImageToolPresentationBinding,
} from '../imageToolBindings'
import ImageToolPresentation from './ImageToolPresentation.vue'

const MaskChromeStub = defineComponent({
  name: 'ImageMaskEditorChrome',
  props: { brushSize: { type: Number, required: true } },
  emits: ['set-brush-size'],
  template: `
    <button data-testid="mask-chrome" @click="$emit('set-brush-size', 48)">
      Mask {{ brushSize }}
    </button>
  `,
})

const EditorChromeStub = defineComponent({
  name: 'ImageEditorChrome',
  props: { activeTool: { type: String, required: true } },
  emits: ['select-tool'],
  template: `
    <button data-testid="editor-chrome" @click="$emit('select-tool', 'brush')">
      Editor {{ activeTool }}
    </button>
  `,
})

function createChromeCommands(): ImageToolChromeCommands {
  return {
    setMaskDrawMode: vi.fn(),
    setMaskBrushShape: vi.fn(),
    setMaskBrushSize: vi.fn(),
    setMaskBrushHardness: vi.fn(),
    setMaskLayerOpacity: vi.fn(),
    clearMask: vi.fn(),
    invertMask: vi.fn(),
    featherMask: vi.fn(),
    undoMask: vi.fn(),
    redoMask: vi.fn(),
    submitMaskPrompt: vi.fn(),
    selectEditorTool: vi.fn(),
    selectAdjustmentLayer: vi.fn(),
    selectEditorLayer: vi.fn(),
    setEditorLayerVisible: vi.fn(),
    addEditorLayer: vi.fn(),
    copyEditorLayer: vi.fn(),
    mergeEditorLayer: vi.fn(),
    removeEditorLayer: vi.fn(),
    updateEditorParam: vi.fn(),
    updateEditorAdjustmentParams: vi.fn(),
    updateEditorRasterToolSettings: vi.fn(),
    undoEditorRasterStroke: vi.fn(),
    redoEditorRasterStroke: vi.fn(),
    applyAdjustments: vi.fn(async () => {}),
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
      brushShape: 'round',
      brushSize: 24,
      brushHardness: 75,
      layerOpacity: 40,
      directGenerating: false,
      canUndo: false,
      canRedo: false,
    },
    editor: {
      activeTool: 'move',
      layers: [],
      selectedLayerId: 'base',
      selectedAdjustmentLayer: null,
      rasterToolSettings: {
        size: 24,
        hardness: 75,
        brushOpacity: 100,
        eraserOpacity: 100,
        brushColor: '#000000',
      },
      canUseRasterTool: false,
      canUndoRasterStroke: false,
      canRedoRasterStroke: false,
      hasRasterEdits: false,
      histogramSource: null,
      applyingAdjustments: false,
    },
    ...overrides,
  }
}

function renderPresentation(model: ImageToolChromeModel, commands = createChromeCommands()) {
  const activeModel = shallowRef(model)
  return {
    ...render(ImageToolPresentation, {
      props: {
        imageTools: {
          viewModel: {
            chrome: computed(() => activeModel.value) as ComputedRef<ImageToolChromeModel>,
          },
          commands: { chrome: commands },
        } satisfies ImageToolPresentationBinding,
      },
      global: {
        stubs: { ImageMaskEditorChrome: MaskChromeStub, ImageEditorChrome: EditorChromeStub },
      },
    }),
    commands,
    async setModel(nextModel: ImageToolChromeModel) {
      activeModel.value = nextModel
      await nextTick()
    },
  }
}

describe('ImageToolPresentation', () => {
  it('renders Mask chrome and dispatches its command through the Image Tool session', async () => {
    const view = renderPresentation(createChromeModel())

    expect(screen.getByTestId('mask-chrome')).toHaveTextContent('Mask 24')
    await screen.getByTestId('mask-chrome').click()
    expect(view.commands.setMaskBrushSize).toHaveBeenCalledWith(48)
  })

  it('renders Editor chrome and dispatches its command through the Image Tool session', async () => {
    const view = renderPresentation(createChromeModel({ mode: 'adjust' }))

    expect(screen.getByTestId('editor-chrome')).toHaveTextContent('Editor move')
    await screen.getByTestId('editor-chrome').click()
    expect(view.commands.selectEditorTool).toHaveBeenCalledWith('brush')
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
