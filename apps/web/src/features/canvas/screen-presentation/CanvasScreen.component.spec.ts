import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { render } from '../../../test/render'
import CanvasScreen from './CanvasScreen.vue'

type ScreenPresentationCapture = {
  chrome: { commands: { exportSelectedImages: () => void; zoomOut: () => void } }
  canvas: { flow: { onNodesChange: (changes: unknown[]) => void; onPaneClick: () => void } }
}

type KeyboardOptions = { onSave: () => void }
type LeaveGuardOptions = { onLeave: () => Promise<unknown> | unknown }
type GroupOptions = { grouping: { onAction: () => void } }

const mocks = vi.hoisted(() => ({
  presentation: null as ScreenPresentationCapture | null,
  keyboardOptions: null as KeyboardOptions | null,
  leaveGuardOptions: null as LeaveGuardOptions | null,
  groupOptions: null as GroupOptions | null,
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

vi.mock('../group-presentation', () => ({
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
}))

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
  createCanvasNodePresentationIntentRouter: () => ({ dispatch: vi.fn() }),
}))

vi.mock('./useCanvasNodeContextMenu', () => ({
  useCanvasNodeContextMenu: () => ({
    nodeContextMenu: ref({ visible: false, x: 0, y: 0, flowX: 0, flowY: 0 }),
    canvasNodeMenuItems: [],
    close: vi.fn(),
    open: vi.fn(),
    createNode: vi.fn(),
  }),
}))

const ScreenPresentationStub = defineComponent({
  props: { presentation: { type: Object, required: true } },
  setup(props) {
    mocks.presentation = props.presentation as ScreenPresentationCapture
    return () => h('section', { 'data-testid': 'canvas-screen-presentation' })
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
          createPromptGenerationHandlers: () => ({ onFlowPromptGenerate: vi.fn() }),
          createImageActionHandlers: () => ({
            onFlowImageContextMenu: vi.fn(),
            onFlowFusionContextMenu: vi.fn(),
            onFlowImageDirectAction: vi.fn(),
            onFlowImagePromptGenerate: vi.fn(),
            onFlowFusionPromptGenerate: vi.fn(),
          }),
        },
        nodeInteraction: {
          createNodeMediaInputs: () => ({
            onFlowImageOpenFile: vi.fn(),
            onFlowImageReset: vi.fn(),
            onFlowImageStageDrop: vi.fn(),
          }),
          createNodeHandlers: () => ({
            onFlowDeleteNode: vi.fn(),
            onFlowPromptTextUpdate: vi.fn(),
            onFlowNodeParamUpdate: vi.fn(),
            onFlowImageReset: vi.fn(),
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

describe('CanvasScreen', () => {
  it('assembles workspace, chrome, group, keyboard, leave guard, and board lifecycle at the Screen seam', async () => {
    const harness = createRuntime()
    mocks.useCanvasWorkspaceRuntime.mockReturnValue(harness.runtime)

    render(CanvasScreen, {
      props: {
        route: { query: {} } as RouteLocationNormalizedLoaded,
        router: {} as Router,
        getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
        logoutAndRedirect: vi.fn(),
      },
      global: { stubs: { CanvasScreenPresentation: ScreenPresentationStub } },
    })

    await Promise.resolve()

    expect(harness.loadCanvasBoardAndProbe).toHaveBeenCalledTimes(1)
    expect(harness.reattachPendingImageResults).toHaveBeenCalledTimes(1)
    mocks.presentation!.chrome.commands.exportSelectedImages()
    mocks.presentation!.chrome.commands.zoomOut()
    mocks.presentation!.canvas.flow.onNodesChange([{ id: 'node-a', type: 'remove' }])
    mocks.presentation!.canvas.flow.onPaneClick()
    mocks.groupOptions!.grouping.onAction()
    mocks.keyboardOptions!.onSave()
    await mocks.leaveGuardOptions!.onLeave()

    expect(harness.onExportSelectedImages).toHaveBeenCalledTimes(1)
    expect(harness.onZoomOut).toHaveBeenCalledTimes(1)
    expect(harness.runtime.commands.flowPane.onNodesChange).toHaveBeenCalledWith([
      { id: 'node-a', type: 'remove' },
    ])
    expect(harness.handlePaneClick).toHaveBeenCalledTimes(1)
    expect(harness.groupSelectedNodes).toHaveBeenCalledTimes(1)
    expect(harness.flushAutosave).toHaveBeenNthCalledWith(1, { silent: false })
    expect(harness.flushAutosave).toHaveBeenNthCalledWith(2)
  })
})
