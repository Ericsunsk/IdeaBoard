/* eslint-disable vue/one-component-per-file -- Local chrome stubs keep the overlay contract test focused. */
import { computed, defineComponent, type ComputedRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import CanvasOverlays from './CanvasOverlays.vue'
import { render, screen } from '../../../test/render'
import type {
  CanvasImageTools,
  ImageToolChromeCommands,
  ImageToolChromeModel,
} from '../image-tools'

type ImageToolChromeSession = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}

const MaskChromeStub = defineComponent({
  name: 'ImageMaskEditorChrome',
  props: {
    anchorRect: { type: Object, required: true },
    brushSize: { type: Number, required: true },
  },
  emits: ['set-brush-size'],
  template: `
    <button data-testid="mask-chrome" @click="$emit('set-brush-size', 48)">
      Mask {{ brushSize }}
    </button>
  `,
})

const EditorChromeStub = defineComponent({
  name: 'ImageEditorChrome',
  props: {
    activeTool: { type: String, required: true },
  },
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
    updateMaskOutputCount: vi.fn(),
    openMaskReferenceImageInput: vi.fn(),
    removeMaskReferenceImage: vi.fn(),
    previewMaskReferenceImage: vi.fn(),
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
      outputCount: 2,
      referenceImages: [],
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

function createImageToolSession(
  model: ImageToolChromeModel,
  commands = createChromeCommands(),
): { session: ImageToolChromeSession; commands: ImageToolChromeCommands } {
  return {
    session: {
      viewModel: {
        chrome: computed(() => model) as ComputedRef<ImageToolChromeModel>,
      },
      commands: {
        chrome: commands,
      },
    },
    commands,
  }
}

function renderOverlays(session: ImageToolChromeSession) {
  return render(CanvasOverlays, {
    props: {
      imageToolSession: session,
      nodeContextMenu: { visible: false, x: 0, y: 0 },
      canvasNodeMenuItems: [],
    },
    global: {
      stubs: {
        ImageMaskEditorChrome: MaskChromeStub,
        ImageEditorChrome: EditorChromeStub,
        CanvasNodeContextMenu: { template: '<div data-testid="node-context-menu" />' },
      },
    },
  })
}

describe('CanvasOverlays image tool chrome wiring', () => {
  it('routes mask chrome model and commands through the image tool session seam', async () => {
    const { session, commands } = createImageToolSession(createChromeModel())

    renderOverlays(session)

    expect(screen.getByTestId('mask-chrome')).toHaveTextContent('Mask 24')

    await screen.getByTestId('mask-chrome').click()

    expect(commands.setMaskBrushSize).toHaveBeenCalledWith(48)
  })

  it('routes editor chrome model and commands through the image tool session seam', async () => {
    const { session, commands } = createImageToolSession(
      createChromeModel({
        mode: 'adjust',
      }),
    )

    renderOverlays(session)

    expect(screen.getByTestId('editor-chrome')).toHaveTextContent('Editor move')

    await screen.getByTestId('editor-chrome').click()

    expect(commands.selectEditorTool).toHaveBeenCalledWith('brush')
  })
})
