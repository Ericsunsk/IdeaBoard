/* eslint-disable vue/one-component-per-file */
import { computed, defineComponent, h, nextTick, ref, type Component, type PropType } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { Edge, Node } from '@vue-flow/core'
import { render } from '../../../test/render'
import type { CanvasGroupPresentationInteraction } from '../group-presentation'
import type { ImageToolChromeCommands, ImageToolChromeModel } from '../image-tools'
import type { ImageNodePresentationBinding } from '../node-presentation'
import type { FlowNodeData } from '../types/flowTypes'
import CanvasScreenPresentation from './CanvasScreenPresentation.vue'

function createScreen(
  overrides: {
    chromeHidden?: boolean
    exportDisabled?: boolean
    generationAvailable?: boolean
  } = {},
) {
  const hidden = ref(overrides.chromeHidden ?? false)
  const locked = ref(false)
  const selected = ref(overrides.exportDisabled === false)
  const activeTool = ref<'select' | 'hand'>('select')
  const shortcutSuppressed = ref(false)
  const generationAvailable = ref(overrides.generationAvailable ?? true)
  const nodes = ref<Node<FlowNodeData>[]>([])
  const edges = ref<Edge[]>([])
  const imagePresentation = {} as ImageNodePresentationBinding
  const group = {} as CanvasGroupPresentationInteraction
  const imageToolSession = {
    viewModel: { chrome: computed(() => ({}) as ImageToolChromeModel) },
    commands: { chrome: {} as ImageToolChromeCommands },
  }

  return {
    state: {
      hidden,
      locked,
      selected,
      activeTool,
      shortcutSuppressed,
      generationAvailable,
    },
    workspace: {
      viewModel: {
        flowPane: { nodes, edges },
        graphEditing: { hasSelectedFlowNodes: computed(() => selected.value) },
        viewportControls: { zoomLevel: computed(() => 100) },
      },
    },
    chrome: {
      board: computed(() => ({
        currentName: '',
        options: [],
        currentId: null,
        isMenuOpen: false,
        isRenaming: false,
        isRenamePending: false,
        catalogRecovery: { status: 'idle' as const },
        setNameInputRef: vi.fn(),
      })),
      account: computed(() => ({
        balance: {
          status: 'ready' as const,
          displayValue: '0 Idea',
          ariaLabel: '账户余额 0 Idea',
          availableIdeas: 0,
          command: vi.fn(),
        },
        menuItems: [],
      })),
      activeTool,
      minimapEnabled: ref(true),
      isHidden: hidden,
      closeBoardHeaderEditing: vi.fn(),
      commands: {
        updateBoardNameDraft: vi.fn(),
        toggleBoardMenu: vi.fn(),
        startBoardRename: vi.fn(),
        cancelBoardRename: vi.fn(),
        confirmBoardRename: vi.fn(),
        reloadBoardCatalog: vi.fn(),
        selectBoard: vi.fn(),
        exportSelectedImages: vi.fn(),
        navigateBack: vi.fn(),
        selectTool: vi.fn(),
        toggleMinimap: vi.fn(),
        zoomOut: vi.fn(),
        zoomIn: vi.fn(),
      },
    },
    canvas: {
      containerRef: ref<HTMLElement | null>(null),
      isInteractionActive: ref(false),
      isViewportPanActive: ref(false),
      isLocked: locked,
      isShortcutSuppressed: computed(() => shortcutSuppressed.value),
      connectionBindings: {
        isValidConnection: vi.fn(),
        onConnect: vi.fn(),
        onEdgeUpdate: vi.fn(),
      },
      surface: {
        onDragOver: vi.fn(),
        onDrop: vi.fn(),
        onPointerDownCapture: vi.fn(),
        onContextMenuCapture: vi.fn(),
        onWheel: vi.fn(),
      },
      flow: {
        onNodesChange: vi.fn(),
        onEdgesChange: vi.fn(),
        onConnectEnd: vi.fn(),
        onMoveStart: vi.fn(),
        onMove: vi.fn(),
        onMoveEnd: vi.fn(),
        onViewportChangeEnd: vi.fn(),
        onNodeClick: vi.fn(),
        onNodeDragStart: vi.fn(),
        onNodeDragStop: vi.fn(),
        onSelectionStart: vi.fn(),
        onSelectionEnd: vi.fn(),
        onPaneClick: vi.fn(),
        onPaneContextMenu: vi.fn(),
      },
      group,
      node: {
        copyMode: ref(false),
        getUiState: vi.fn(),
        imagePresentationFor: () => imagePresentation,
        fusionPresentationFor: vi.fn(),
        generationAvailable,
        setImageStageRef: vi.fn(),
        clearImageStageRef: vi.fn(),
        expand: {
          nodeId: ref<string | null>(null),
          overlayStyle: ref<Record<string, string> | null>(null),
          handleRadius: 4,
          visibilityRefreshToken: ref(0),
        },
        commands: {
          updatePromptText: vi.fn(),
          updateNodeParameter: vi.fn(),
          deleteNode: vi.fn(),
          dropMediaInput: vi.fn(),
          openMediaInput: vi.fn(),
          previewMediaInput: vi.fn(),
          removeMediaInput: vi.fn(),
          generateFromPromptNode: vi.fn(),
          dropImageStage: vi.fn(),
          openImageFile: vi.fn(),
          previewImage: vi.fn(),
          appendImageToCanvas: vi.fn(),
          resetImage: vi.fn(),
          openImageContextMenu: vi.fn(),
          generateFromImagePrompt: vi.fn(),
          submitCanvasExtend: vi.fn(),
          runImageAction: vi.fn(),
          beginCanvasExtendHandle: vi.fn(),
          moveCanvasExtendHandle: vi.fn(),
          endCanvasExtendHandle: vi.fn(),
          beginCanvasExtendFrame: vi.fn(),
          openFusionContextMenu: vi.fn(),
          generateFromFusion: vi.fn(),
        },
        listeners: {
          nodeMousedown: vi.fn(),
          requestInternalsUpdate: vi.fn(),
        },
      },
      minimap: {
        enabled: ref(true),
        isHidden: hidden,
        commands: {
          centerRequested: vi.fn(),
          dragStart: vi.fn(),
          dragMove: vi.fn(),
          dragEnd: vi.fn(),
        },
      },
      overlays: {
        imageToolSession,
        nodeContextMenu: ref({ visible: false, x: 0, y: 0 }),
        canvasNodeMenuItems: [],
        createNode: vi.fn(),
      },
    },
  }
}

const VueFlowStub = defineComponent({
  props: {
    panOnDrag: Boolean,
    nodesDraggable: Boolean,
    deleteKeyCode: {
      type: [Array, String, null] as PropType<string[] | string | null>,
      default: null,
    },
  },
  emits: ['nodes-change'],
  setup(props, { emit, slots }) {
    return () =>
      h(
        'section',
        {
          'data-testid': 'flow-region',
          'data-pan-on-drag': `${props.panOnDrag}`,
          'data-nodes-draggable': `${props.nodesDraggable}`,
          'data-delete-enabled': `${props.deleteKeyCode !== null}`,
        },
        [
          h(
            'button',
            {
              type: 'button',
              onClick: () => emit('nodes-change', [{ id: 'node-1', type: 'remove' }]),
            },
            '触发节点变更',
          ),
          slots['zoom-pane']?.(),
          slots['node-canvas']?.({ id: 'node-1', data: {}, selected: false, dragging: false }),
          slots.default?.(),
        ],
      )
  },
})

const ChromeStub = defineComponent({
  props: {
    inert: Boolean,
    isHidden: Boolean,
    status: { type: Object as PropType<{ exportDisabled: boolean }>, required: true },
  },
  setup(props) {
    return () =>
      h('section', {
        'data-testid': 'chrome-region',
        'data-hidden': `${props.isHidden}`,
        'data-inert': `${props.inert}`,
        'data-export-disabled': `${props.status.exportDisabled}`,
      })
  },
})

const GroupStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'group-region' })
  },
})

const NodeStub = defineComponent({
  props: { generationAvailable: Boolean },
  emits: ['node-intent', 'node-mousedown', 'request-internals-update'],
  setup(props, { emit }) {
    return () =>
      h(
        'section',
        {
          'data-testid': 'node-region',
          'data-generation-available': `${props.generationAvailable}`,
        },
        [
          h(
            'button',
            {
              type: 'button',
              onClick: () => {
                emit('node-intent', {
                  type: 'delete-requested',
                  nodeId: 'node-1',
                })
                emit('node-mousedown', 'node-1', new MouseEvent('mousedown'))
                emit('request-internals-update', 'node-1')
              },
            },
            '触发节点事件',
          ),
        ],
      )
  },
})

const MinimapStub = defineComponent({
  props: { enabled: Boolean, inert: Boolean, isHidden: Boolean },
  setup(props) {
    return () =>
      h('section', {
        'data-testid': 'minimap-region',
        'data-enabled': `${props.enabled}`,
        'data-hidden': `${props.isHidden}`,
      })
  },
})

const OverlaysStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'overlay-region' })
  },
})

function renderPresentation(input = {}) {
  const presentation = createScreen(input)
  const view = render(CanvasScreenPresentation as Component, {
    props: { presentation },
    global: {
      stubs: {
        CanvasChrome: ChromeStub,
        VueFlow: VueFlowStub,
        CanvasGroupPresentation: GroupStub,
        FlowCanvasNode: NodeStub,
        CanvasChromeMiniMap: MinimapStub,
        CanvasOverlays: OverlaysStub,
      },
    },
  })

  return { presentation, ...view }
}

describe('CanvasScreenPresentation', () => {
  it('assembles the Chrome, flow, node, group, minimap, and overlay regions', () => {
    const view = renderPresentation()

    expect(view.getByTestId('chrome-region')).toBeInTheDocument()
    expect(view.getByTestId('flow-region')).toBeInTheDocument()
    expect(view.getByTestId('node-region')).toHaveAttribute('data-generation-available', 'true')
    expect(view.getByTestId('group-region')).toBeInTheDocument()
    expect(view.getByTestId('minimap-region')).toHaveAttribute('data-enabled', 'true')
    expect(view.getByTestId('overlay-region')).toBeInTheDocument()
  })

  it('reactively presents Chrome, generation, and Vue Flow interaction policy', async () => {
    const view = renderPresentation({
      exportDisabled: true,
    })

    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-hidden', 'false')
    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-export-disabled', 'true')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-pan-on-drag', 'false')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-nodes-draggable', 'true')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-delete-enabled', 'true')

    view.presentation.state.hidden.value = true
    view.presentation.state.selected.value = true
    view.presentation.state.activeTool.value = 'hand'
    view.presentation.state.shortcutSuppressed.value = true
    view.presentation.state.generationAvailable.value = false
    await nextTick()

    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-hidden', 'true')
    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-inert', 'true')
    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-export-disabled', 'false')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-pan-on-drag', 'true')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-nodes-draggable', 'false')
    expect(view.getByTestId('flow-region')).toHaveAttribute('data-delete-enabled', 'false')
    expect(view.getByTestId('node-region')).toHaveAttribute('data-generation-available', 'false')
    expect(view.getByTestId('minimap-region')).toHaveAttribute('data-hidden', 'true')
  })

  it('forwards Vue Flow node changes to the semantic flow command', () => {
    const view = renderPresentation()

    view.getByRole('button', { name: '触发节点变更' }).click()

    expect(view.presentation.canvas.flow.onNodesChange).toHaveBeenCalledWith([
      { id: 'node-1', type: 'remove' },
    ])
  })

  it('dispatches Canvas Node intents to semantic commands', () => {
    const view = renderPresentation()
    const mouseEvent = expect.any(MouseEvent)

    view.getByRole('button', { name: '触发节点事件' }).click()

    expect(view.presentation.canvas.node.commands.deleteNode).toHaveBeenCalledWith('node-1')
    expect(view.presentation.canvas.node.listeners.nodeMousedown).toHaveBeenCalledWith(
      'node-1',
      mouseEvent,
    )
    expect(view.presentation.canvas.node.listeners.requestInternalsUpdate).toHaveBeenCalledWith(
      'node-1',
    )
  })
})
