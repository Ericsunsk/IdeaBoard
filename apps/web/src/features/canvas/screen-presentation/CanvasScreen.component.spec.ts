/* eslint-disable vue/one-component-per-file */
import { defineComponent, h, ref, type PropType } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { render } from '../../../test/render'
import CanvasScreen from './CanvasScreen.vue'

type KeyboardOptions = { onSave: () => void }
type LeaveGuardOptions = { onLeave: () => Promise<unknown> | unknown }
type GroupOptions = { grouping: { onAction: () => void } }

const mocks = vi.hoisted(() => ({
  keyboardOptions: null as KeyboardOptions | null,
  leaveGuardOptions: null as LeaveGuardOptions | null,
  groupOptions: null as GroupOptions | null,
  nodeIntentDispatch: vi.fn(),
  createContextMenuNode: vi.fn(),
  onFlowNodeMouseDown: vi.fn(),
  useCanvasWorkspaceRuntime: vi.fn(),
}))

vi.mock('../workspace-runtime', () => ({
  useCanvasWorkspaceRuntime: mocks.useCanvasWorkspaceRuntime,
}))

vi.mock('../flow-pane', () => ({
  useVueFlowPaneAdapter: () => ({
    screenToFlowCoordinate: vi.fn((point: { x: number; y: number }) => point),
    viewport: ref({ x: 0, y: 0, zoom: 1 }),
  }),
  useFlowPaneSelectionAdapter: () => ({
    paneSelectionBindings: { onPointerDownCapture: vi.fn() },
    vueFlowSelectionBindings: {
      onEdgesChange: vi.fn(),
      onSelectionStart: vi.fn(),
      onSelectionEnd: vi.fn(),
    },
    commands: {
      clearSelection: vi.fn(() => true),
      onFlowNodeClick: vi.fn(),
      onFlowNodeMouseDown: mocks.onFlowNodeMouseDown,
      selectNodeForContextMenu: vi.fn(),
    },
  }),
}))

vi.mock('../composables/useFlowKeyboardShortcuts', () => ({
  useFlowKeyboardShortcuts: (options: KeyboardOptions) => {
    mocks.keyboardOptions = options
  },
}))

vi.mock('../composables/useFlowLeaveGuard', () => ({
  useFlowLeaveGuard: (options: LeaveGuardOptions) => {
    mocks.leaveGuardOptions = options
  },
}))

vi.mock('../composables/useFlowImageActionStrengthControl', () => ({
  useFlowImageActionStrengthControl: () => ({
    activeStrengthAction: ref(null),
    consumeSuppressedStrengthClick: vi.fn(),
    setStrengthRangeRef: vi.fn(),
    setStrengthTriggerRef: vi.fn(),
    hideStrengthControl: vi.fn(),
    getActionStrength: vi.fn(),
    onStrengthRangePointerDown: vi.fn(),
    onStrengthInput: vi.fn(),
    onStrengthChange: vi.fn(),
    onStrengthPointerUp: vi.fn(),
    onStrengthClick: vi.fn(),
  }),
}))

vi.mock('../composables/useFlowNodeDragInteractions', () => ({
  useFlowNodeDragInteractions: () => ({ onNodeDragStart: vi.fn(), onNodeDragStop: vi.fn() }),
}))

vi.mock('../composables/useFlowImagePreviewLightbox', () => ({
  useFlowImagePreviewLightbox: () => ({
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
  }),
}))

vi.mock('../composables/useCanvasViewportContext', () => ({
  provideCanvasViewportContext: vi.fn(),
}))

vi.mock('../composables/useFlowActiveNodePanels', () => ({
  ACTIVE_NODE_PANEL_KIND: { imageTools: 'imageTools', fusionPrompt: 'fusionPrompt' },
  useFlowActiveNodePanels: () => ({
    activeNodePanel: ref(null),
    openActiveNodePanel: vi.fn(),
    clearActiveNodePanel: vi.fn(),
    isActiveNodePanel: vi.fn(() => false),
    getActiveNodePanelNodeId: vi.fn(() => null),
  }),
}))

vi.mock('../composables/useFlowCanvasShellInteractions', () => ({
  useFlowCanvasShellInteractions: () => ({
    onPaneContextMenu: vi.fn(),
    setActiveTool: vi.fn(),
    onEscapeShortcut: vi.fn(() => false),
    onCanvasDragOver: vi.fn(),
  }),
}))

vi.mock('../group-presentation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../group-presentation')>()
  return {
    ...actual,
    useCanvasGroupPresentationInteraction: (options: GroupOptions) => {
      mocks.groupOptions = options
      return {
        bindClosePolicyHandlers: <T extends Record<string, unknown>>(handlers: T) => ({
          ...handlers,
          onCanvasPaneClick: handlers.onCanvasPaneClick,
        }),
        onCanvasContextMenuCapture: vi.fn(),
        onCanvasPointerDownCapture: vi.fn(),
      }
    },
  }
})

vi.mock('../composables/useFlowExpandInteractionHandlers', () => ({
  useFlowExpandInteractionHandlers: () => ({
    onFlowImageExpandSubmit: vi.fn(),
    onFlowExpandHandlePointerDown: vi.fn(),
    onFlowExpandFramePointerDown: vi.fn(),
  }),
}))

vi.mock('../composables/useFlowTransientUiDismissal', () => ({
  useFlowTransientUiDismissal: () => ({ dismissTransientCanvasUi: vi.fn() }),
}))

vi.mock('../composables/useImageStageRegistry', () => ({
  useImageStageRegistry: () => ({
    setImageStageRef: vi.fn(),
    clearImageStageRef: vi.fn(),
    getImageStageElement: vi.fn(() => null),
  }),
}))

vi.mock('../node-presentation', () => ({
  createImageNodePresentationResolver: () => ({ forNode: vi.fn() }),
  createFusionNodePresentationResolver: () => ({ forNode: vi.fn() }),
}))

vi.mock('../overlay-lifecycle', () => ({ useCanvasCustomLayerOverlay: vi.fn() }))

vi.mock('../../user-shell/useUserAccountControls', () => ({
  useUserAccountControls: () => ({
    viewModel: { avatarUrl: ref(null), balance: ref({}), menuItems: ref([]) },
    commands: { openAccount: vi.fn() },
  }),
}))

vi.mock('./createCanvasNodePresentationIntentRouter', () => ({
  createCanvasNodePresentationIntentRouter: () => ({ dispatch: mocks.nodeIntentDispatch }),
}))

vi.mock('./useCanvasNodeContextMenu', () => ({
  useCanvasNodeContextMenu: () => ({
    nodeContextMenu: ref({ visible: false, x: 0, y: 0, flowX: 0, flowY: 0 }),
    canvasNodeMenuItems: [],
    close: vi.fn(),
    open: vi.fn(),
    createNode: mocks.createContextMenuNode,
  }),
}))

const ChromeStub = defineComponent({
  emits: ['export', 'zoom-out'],
  setup(_, { emit }) {
    return () =>
      h('section', { 'data-testid': 'chrome-region' }, [
        h('button', { type: 'button', onClick: () => emit('export') }, '导出选中图片'),
        h('button', { type: 'button', onClick: () => emit('zoom-out') }, '缩小画布'),
      ])
  },
})

const VueFlowStub = defineComponent({
  emits: ['nodes-change', 'pane-click'],
  setup(_, { emit, slots }) {
    return () =>
      h('section', { 'data-testid': 'flow-region' }, [
        h(
          'button',
          {
            type: 'button',
            onClick: () => emit('nodes-change', [{ id: 'node-a', type: 'remove' }]),
          },
          '删除节点',
        ),
        h('button', { type: 'button', onClick: () => emit('pane-click') }, '点击画布'),
        slots['zoom-pane']?.(),
        slots['node-canvas']?.({ id: 'node-a', data: {}, selected: false, dragging: false }),
        slots.default?.(),
      ])
  },
})

const GroupStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'group-region' })
  },
})

const NodeStub = defineComponent({
  emits: ['node-intent', 'node-mousedown', 'request-internals-update'],
  setup(_, { emit }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          'data-testid': 'node-region',
          onClick: () => {
            emit('node-intent', { type: 'delete-requested', nodeId: 'node-a' })
            emit('node-mousedown', 'node-a', new MouseEvent('mousedown'))
            emit('request-internals-update', 'node-a')
          },
        },
        '触发节点交互',
      )
  },
})

const MiniMapStub = defineComponent({
  emits: ['center-requested'],
  setup(_, { emit }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          'data-testid': 'minimap-region',
          onClick: () => emit('center-requested', { position: { x: 10, y: 20 } }),
        },
        '居中小地图',
      )
  },
})

const OverlaysStub = defineComponent({
  props: {
    nodeContextMenu: {
      type: Object as PropType<{ visible: boolean }>,
      required: true,
    },
  },
  emits: ['create-node'],
  setup(_, { emit }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          'data-testid': 'overlay-region',
          onClick: () => emit('create-node', 'prompt'),
        },
        '创建提示词节点',
      )
  },
})

function createRuntime() {
  const flushAutosave = vi.fn()
  const loadCanvasBoardAndProbe = vi.fn(async () => {})
  const reattachPendingImageResults = vi.fn()
  const onZoomOut = vi.fn()
  const onExportSelectedImages = vi.fn()
  const handlePaneClick = vi.fn()
  const groupSelectedNodes = vi.fn()
  const selectionGroupAction = ref({ kind: 'group' as const })

  return {
    runtime: {
      viewModel: {
        flowPane: { nodes: ref([]), edges: ref([]) },
        boardChrome: {
          currentBoardId: ref('board-a'),
          isBoardMenuOpen: ref(false),
          isRenamingBoard: ref(false),
          boardNameDraft: ref('Board A'),
          isRenamePending: ref(false),
          catalogRecovery: ref({ status: 'idle' }),
          currentBoardName: ref('Board A'),
          boardOptions: ref([]),
        },
        persistence: { history: { undo: vi.fn(), redo: vi.fn() } },
        viewportControls: { zoomLevel: ref(100) },
        graphEditing: {
          hasSelectedFlowNodes: ref(false),
          isFlowInteractionActive: ref(false),
          isFlowViewportPanInteractionActive: ref(false),
          isFlowInteractionLocked: ref(false),
          grouping: { groupFrames: ref([]), selectionGroupAction },
        },
      },
      commands: {
        boardSession: {
          setBoardNameInputRef: vi.fn(),
          toggleBoardMenu: vi.fn(),
          startBoardRename: vi.fn(),
          cancelBoardRename: vi.fn(),
          closeBoardHeaderEditing: vi.fn(),
          onConfirmBoardRename: vi.fn(),
          reloadCatalog: vi.fn(),
          onSelectBoard: vi.fn(),
          onNavigateRequest: vi.fn(),
          onLogoutRequest: vi.fn(),
        },
        flowPane: {
          onNodesChange: vi.fn(),
          onEdgesChange: vi.fn(),
          connectionAdapter: { vueFlowBindings: {}, connectIntent: vi.fn() },
          nodeDragTransactions: { begin: vi.fn(), commit: vi.fn() },
        },
        persistence: { flushAutosave, loadCanvasBoardAndProbe, reattachPendingImageResults },
        viewportControls: {
          onZoomIn: vi.fn(),
          onZoomOut,
          onPanViewport: vi.fn(),
          onMiniMapCenterRequested: vi.fn(),
          onMiniMapDragStart: vi.fn(),
          onMiniMapDragMove: vi.fn(),
          onMiniMapDragEnd: vi.fn(),
          onViewportChangeEnd: vi.fn(),
          onFlowMoveStart: vi.fn(),
          onFlowMove: vi.fn(),
          onFlowMoveEnd: vi.fn(),
          onNodeDragStart: vi.fn(),
          onNodeDragStop: vi.fn(),
        },
        graphEditing: {
          grouping: {
            selectGroupAtPoint: vi.fn(),
            handlePaneClick,
            handleNodeClick: vi.fn(),
            groupSelectedNodes,
            ungroupSelectedGroup: vi.fn(),
            deleteSelectedGroup: vi.fn(() => false),
            groupDrag: vi.fn(),
            groupResize: vi.fn(),
          },
          scheduleFlowNodeInternalsRefresh: vi.fn(),
          createNodeAtPosition: vi.fn(),
          onExportSelectedImages,
          onFlowMediaInputImageDrop: vi.fn(),
          onFlowMediaInputImageOpen: vi.fn(),
          onFlowMediaInputImagePreview: vi.fn(),
          onFlowMediaInputImageRemove: vi.fn(),
          onCanvasDrop: vi.fn(),
          onCanvasDragOver: vi.fn(),
          onFlowImageOpenPreview: vi.fn(),
          onFlowImageAppendToCanvas: vi.fn(),
          beginFlowNodeDragDuplicateTracking: vi.fn(),
          finishFlowNodeDragDuplicateTracking: vi.fn(),
          getFlowNodeUiStateCached: vi.fn(),
          createNodeInteractions: () => ({
            onFlowDeleteNode: vi.fn(),
            onFlowPromptTextUpdate: vi.fn(),
            onFlowNodeParamUpdate: vi.fn(),
            onFlowImageOpenFile: vi.fn(),
            onFlowImageReset: vi.fn(),
            onFlowImageStageDrop: vi.fn(),
          }),
        },
        imageWorkflow: {
          isGenerationAvailable: vi.fn(() => true),
          runImageDirectAction: vi.fn(),
          createCanvasExtendOverlay: () => ({
            expandNodeId: ref(null),
            expandOverlayStyle: ref(null),
            enterExpandMode: vi.fn(),
            cancelExpandMode: vi.fn(),
            beginHandleResize: vi.fn(),
            beginFrameDrag: vi.fn(),
            onHandlePointerMove: vi.fn(),
            onHandlePointerUp: vi.fn(),
            commitExpandMode: vi.fn(),
            handleRadius: 4,
          }),
          createImageTools: () => ({
            viewModel: {
              active: { nodeId: ref(null), maskNodeId: ref(null) },
              locks: { isChromeHidden: ref(false), isCanvasLocked: ref(false) },
              nodeBindings: { stageFor: vi.fn(), actionFor: vi.fn() },
              chrome: ref({ isReady: false }),
            },
            commands: {
              lifecycle: { enter: vi.fn(), cancel: vi.fn(), discardNodeEdits: vi.fn() },
              editor: {
                onCanvasWheel: vi.fn(),
                focusAndShowChrome: vi.fn(),
                scheduleChromeAnchorUpdate: vi.fn(),
              },
              prompt: { runPromptAction: vi.fn() },
              chrome: {},
            },
          }),
          createImageActionHandlers: () => ({
            onFlowImageContextMenu: vi.fn(),
            onFlowFusionContextMenu: vi.fn(),
            onFlowImageDirectAction: vi.fn(),
            onFlowImagePromptGenerate: vi.fn(),
            onFlowFusionPromptGenerate: vi.fn(),
            onFlowPromptGenerate: vi.fn(),
          }),
        },
      },
    },
    flushAutosave,
    loadCanvasBoardAndProbe,
    reattachPendingImageResults,
    onZoomOut,
    onExportSelectedImages,
    handlePaneClick,
    groupSelectedNodes,
  }
}

function renderScreen() {
  const harness = createRuntime()
  mocks.useCanvasWorkspaceRuntime.mockReturnValue(harness.runtime)

  const view = render(CanvasScreen, {
    props: {
      route: { query: {} } as RouteLocationNormalizedLoaded,
      router: {} as Router,
      getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
      logoutAndRedirect: vi.fn(),
    },
    global: {
      stubs: {
        CanvasChrome: ChromeStub,
        VueFlow: VueFlowStub,
        CanvasGroupPresentation: GroupStub,
        FlowCanvasNode: NodeStub,
        CanvasChromeMiniMap: MiniMapStub,
        CanvasOverlays: OverlaysStub,
      },
    },
  })

  return { harness, ...view }
}

describe('CanvasScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.keyboardOptions = null
    mocks.leaveGuardOptions = null
    mocks.groupOptions = null
  })

  it('renders and forwards Canvas interactions through the public Screen seam', () => {
    const view = renderScreen()
    const { harness } = view

    expect(view.getByTestId('chrome-region')).toBeInTheDocument()
    expect(view.getByTestId('flow-region')).toBeInTheDocument()
    expect(view.getByTestId('node-region')).toBeInTheDocument()
    expect(view.getByTestId('group-region')).toBeInTheDocument()
    expect(view.getByTestId('minimap-region')).toBeInTheDocument()
    expect(view.getByTestId('overlay-region')).toBeInTheDocument()

    view.getByRole('button', { name: '导出选中图片' }).click()
    view.getByRole('button', { name: '缩小画布' }).click()
    view.getByRole('button', { name: '删除节点' }).click()
    view.getByRole('button', { name: '点击画布' }).click()
    view.getByRole('button', { name: '触发节点交互' }).click()
    view.getByRole('button', { name: '居中小地图' }).click()
    view.getByRole('button', { name: '创建提示词节点' }).click()
    mocks.groupOptions!.grouping.onAction()

    expect(harness.onExportSelectedImages).toHaveBeenCalledTimes(1)
    expect(harness.onZoomOut).toHaveBeenCalledTimes(1)
    expect(harness.runtime.commands.flowPane.onNodesChange).toHaveBeenCalledWith([
      { id: 'node-a', type: 'remove' },
    ])
    expect(harness.handlePaneClick).toHaveBeenCalledTimes(1)
    expect(mocks.nodeIntentDispatch).toHaveBeenCalledWith({
      type: 'delete-requested',
      nodeId: 'node-a',
    })
    expect(mocks.onFlowNodeMouseDown).toHaveBeenCalledWith('node-a', expect.any(MouseEvent))
    expect(
      harness.runtime.commands.graphEditing.scheduleFlowNodeInternalsRefresh,
    ).toHaveBeenCalledWith(['node-a'])
    expect(harness.runtime.commands.viewportControls.onMiniMapCenterRequested).toHaveBeenCalledWith(
      {
        position: { x: 10, y: 20 },
      },
    )
    expect(mocks.createContextMenuNode).toHaveBeenCalledWith(
      'prompt',
      harness.runtime.commands.graphEditing.createNodeAtPosition,
    )
    expect(harness.groupSelectedNodes).toHaveBeenCalledTimes(1)
  })

  it('loads the Board and reattaches pending results on mount', async () => {
    const { harness } = renderScreen()

    await Promise.resolve()

    expect(harness.loadCanvasBoardAndProbe).toHaveBeenCalledTimes(1)
    expect(harness.reattachPendingImageResults).toHaveBeenCalledTimes(1)
  })

  it('saves through keyboard and leave-guard behavior', async () => {
    const { harness } = renderScreen()

    mocks.keyboardOptions!.onSave()
    await mocks.leaveGuardOptions!.onLeave()

    expect(harness.flushAutosave).toHaveBeenNthCalledWith(1, { silent: false })
    expect(harness.flushAutosave).toHaveBeenNthCalledWith(2)
  })
})
