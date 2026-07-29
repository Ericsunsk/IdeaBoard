import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { Edge, Node, NodeProps } from '@vue-flow/core'
import type { CanvasGroupPresentationInteraction } from '../group-presentation'
import type { ImageNodePresentationBinding } from '../node-presentation'
import type { ImageToolChromeCommands, ImageToolChromeModel } from '../image-tools'
import type { FlowNodeData } from '../types/flowTypes'
import { useCanvasScreenPresentation, type CanvasScreenPresentationOptions } from './index'

function createOptions() {
  const hidden = ref(false)
  const locked = ref(false)
  const selected = ref(false)
  const zoomLevel = ref(100)
  const generationAvailable = ref(true)
  const nodes = ref<Node<FlowNodeData>[]>([])
  const edges = ref<Edge[]>([])
  const imagePresentation = {} as ImageNodePresentationBinding
  const group = {} as CanvasGroupPresentationInteraction

  const options = {
    workspace: {
      viewModel: {
        flowPane: { nodes, edges },
        graphEditing: { hasSelectedFlowNodes: computed(() => selected.value) },
        viewportControls: { zoomLevel: computed(() => zoomLevel.value) },
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
      activeTool: ref<'select' | 'hand'>('select'),
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
      isShortcutSuppressed: computed(() => false),
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
        listeners: { intent: vi.fn(), nodeMousedown: vi.fn(), requestInternalsUpdate: vi.fn() },
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
        imageToolSession: {
          viewModel: { chrome: computed(() => ({}) as ImageToolChromeModel) },
          commands: { chrome: {} as ImageToolChromeCommands },
        },
        nodeContextMenu: ref({ visible: false, x: 0, y: 0 }),
        canvasNodeMenuItems: [],
        createNode: vi.fn(),
      },
    },
  }

  return {
    options: options as CanvasScreenPresentationOptions,
    hidden,
    locked,
    selected,
    generationAvailable,
  }
}

describe('useCanvasScreenPresentation', () => {
  it('reactively projects hidden, selected/locked, and generation states', () => {
    const fixture = createOptions()
    const presentation = useCanvasScreenPresentation(fixture.options)
    const node = { id: 'node-1', data: {} } as NodeProps<FlowNodeData>

    expect(presentation.value.chrome.props.inert).toBe(false)
    expect(presentation.value.chrome.props.status.exportDisabled).toBe(true)
    expect(presentation.value.canvas.props.nodesDraggable).toBe(true)
    expect(presentation.value.canvas.node.propsFor(node).generationAvailable).toBe(true)

    fixture.hidden.value = true
    fixture.selected.value = true
    fixture.locked.value = true
    fixture.generationAvailable.value = false

    expect(presentation.value.chrome.props.inert).toBe(true)
    expect(presentation.value.canvas.minimap.props).toMatchObject({ inert: true, isHidden: true })
    expect(presentation.value.chrome.props.status.exportDisabled).toBe(false)
    expect(presentation.value.canvas.props.nodesDraggable).toBe(false)
    expect(presentation.value.canvas.props.deleteKeyCode).toBeNull()
    expect(presentation.value.canvas.node.propsFor(node).generationAvailable).toBe(false)
  })
})
