/* eslint-disable vue/one-component-per-file */
import { defineComponent, h, reactive, ref, type PropType } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { render } from '../../../test/render'
import CanvasScreen from './CanvasScreen.vue'

type KeyboardOptions = {
  onSave: () => void
  onDeleteInlineSelection: () => boolean
}
type LeaveGuardOptions = { onLeave: () => Promise<unknown> | unknown }
type GroupOptions = { grouping: { onAction: () => void } }

const mocks = vi.hoisted(() => ({
  keyboardOptions: null as KeyboardOptions | null,
  leaveGuardOptions: null as LeaveGuardOptions | null,
  groupOptions: null as GroupOptions | null,
  onFlowNodeMouseDown: vi.fn(),
  dragEvents: [] as string[],
  useCanvasWorkspaceRuntime: vi.fn(),
}))

vi.mock('../workspace-runtime', () => ({
  useCanvasWorkspaceRuntime: mocks.useCanvasWorkspaceRuntime,
}))

vi.mock('../flow-pane', () => ({
  createCanvasConnectionDropLifecycle: () => ({
    onConnectStart: vi.fn(),
    onConnectEnd: vi.fn(),
    cancel: vi.fn(),
    dispose: vi.fn(),
  }),
  useVueFlowPaneAdapter: () => ({
    screenToFlowCoordinate: vi.fn((point: { x: number; y: number }) => point),
    viewport: ref({ x: 0, y: 0, zoom: 1 }),
    selectOnlyNode: vi.fn(() => true),
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

vi.mock('../composables/useFlowNodeDragInteractions', () => ({
  useFlowNodeDragInteractions: () => ({
    onNodeDragStart: () => mocks.dragEvents.push('drag:start'),
    onNodeDragStop: () => mocks.dragEvents.push('drag:stop'),
  }),
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

vi.mock('../overlay-lifecycle', () => ({ useCanvasCustomLayerOverlay: vi.fn() }))

vi.mock('../../user-shell/useUserAccountControls', () => ({
  useUserAccountControls: () => ({
    viewModel: { avatarUrl: ref(null), balance: ref({}), menuItems: ref([]) },
    commands: { openAccount: vi.fn() },
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
  emits: ['nodes-change', 'pane-click', 'pane-context-menu', 'node-drag-start', 'node-drag-stop'],
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
        h(
          'button',
          {
            type: 'button',
            onClick: () =>
              emit(
                'pane-context-menu',
                new MouseEvent('contextmenu', { clientX: 12, clientY: 24 }),
              ),
          },
          '打开节点菜单',
        ),
        h('button', { type: 'button', onClick: () => emit('node-drag-start') }, '开始拖动节点'),
        h('button', { type: 'button', onClick: () => emit('node-drag-stop') }, '结束拖动节点'),
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
  emits: ['node-intent', 'node-mousedown'],
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

const ImageToolPresentationStub = defineComponent({
  props: { imageTools: { type: Object, required: true } },
  setup(_, { attrs }) {
    return () =>
      h('section', {
        'data-testid': 'image-tool-presentation',
        'data-forwarded-attrs': Object.keys(attrs).join(','),
      })
  },
})

const ContextMenuStub = defineComponent({
  props: {
    visible: { type: Boolean, required: true },
    x: { type: Number, required: true },
    y: { type: Number, required: true },
    items: { type: Array as PropType<Array<{ key: string }>>, required: true },
  },
  emits: ['create'],
  setup(props, { emit }) {
    return () =>
      props.visible
        ? h(
            'button',
            {
              type: 'button',
              'data-testid': 'node-context-menu',
              onClick: () => emit('create', 'prompt'),
            },
            '创建提示词节点',
          )
        : null
  },
})

function createRuntime() {
  const flushAutosave = vi.fn()
  const recoverBoardSession = vi.fn(async () => {})
  const onZoomOut = vi.fn()
  const onExportSelectedImages = vi.fn()
  const handlePaneClick = vi.fn()
  const groupSelectedNodes = vi.fn()
  const deleteSelectedGroup = vi.fn(() => false)
  const onFlowDeleteNode = vi.fn()
  const selectionGroupAction = ref({ kind: 'group' as const })
  const imageToolsViewModel = {
    active: { nodeId: ref(null), maskNodeId: ref(null) },
    locks: { isChromeHidden: ref(false), isCanvasLocked: ref(false) },
    nodeBindings: {
      stageFor: vi.fn(() => null),
      actionFor: vi.fn((nodeId: string) => ({ nodeId })),
    },
    chrome: ref({ isReady: false }),
  }
  const imageToolCommands = {
    lifecycle: { enter: vi.fn(), cancel: vi.fn(), discardNodeEdits: vi.fn() },
    editor: {
      onCanvasWheel: vi.fn(),
      focusAndShowChrome: vi.fn(),
      scheduleChromeAnchorUpdate: vi.fn(),
    },
    chrome: {},
  }
  const imageWorkflowInteractions = {
    onFlowImageContextMenu: vi.fn(),
    onFlowFusionContextMenu: vi.fn(),
    onFlowImageDirectAction: vi.fn(),
    onFlowImagePromptGenerate: vi.fn(),
    onFlowFusionPromptGenerate: vi.fn(),
    onFlowPromptGenerate: vi.fn(),
    onFlowImageExpandSubmit: vi.fn(),
    onFlowExpandHandlePointerDown: vi.fn(),
    onFlowExpandFramePointerDown: vi.fn(),
    onFlowExpandHandlePointerMove: vi.fn(),
    onFlowExpandHandlePointerUp: vi.fn(),
  }

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
        imageWorkflow: {
          tools: imageToolsViewModel,
          canvasExtend: { nodeId: ref(null), overlayStyle: ref(null), handleRadius: 4 },
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
        persistence: {
          flushAutosave,
          recoverBoardSession,
        },
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
          onNodeDragStart: vi.fn(() => mocks.dragEvents.push('viewport:start')),
          onNodeDragStop: vi.fn(() => mocks.dragEvents.push('viewport:stop')),
        },
        graphEditing: {
          libraryImport: { add: vi.fn() },
          imageImport: { retry: vi.fn(async () => false) },
          grouping: {
            selectGroupAtPoint: vi.fn(),
            handlePaneClick,
            handleNodeClick: vi.fn(),
            groupSelectedNodes,
            ungroupSelectedGroup: vi.fn(),
            deleteSelectedGroup,
            groupDrag: vi.fn(),
            groupResize: vi.fn(),
          },
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
          createNodeInteractions: (interactionOptions: {
            dismissTransientCanvasUi: (options: { closeContextMenu?: boolean }) => void
          }) => ({
            onFlowDeleteNode: (nodeId: string) => {
              interactionOptions.dismissTransientCanvasUi({ closeContextMenu: true })
              onFlowDeleteNode(nodeId)
            },
            onFlowPromptTextUpdate: vi.fn(),
            onFlowNodeParamUpdate: vi.fn(),
            onFlowImageOpenFile: vi.fn(),
            onFlowImageReset: vi.fn(),
            onFlowImageStageDrop: vi.fn(),
          }),
        },
        imageWorkflow: {
          isGenerationAvailable: vi.fn(() => true),
          tools: imageToolCommands,
          interactions: imageWorkflowInteractions,
          cancelCanvasExtend: vi.fn(),
          hideStrengthControl: vi.fn(),
        },
      },
    },
    flushAutosave,
    recoverBoardSession,
    onZoomOut,
    onExportSelectedImages,
    handlePaneClick,
    groupSelectedNodes,
    deleteSelectedGroup,
    onFlowDeleteNode,
  }
}

function renderScreen(
  route: RouteLocationNormalizedLoaded = { query: {} } as RouteLocationNormalizedLoaded,
) {
  const harness = createRuntime()
  mocks.useCanvasWorkspaceRuntime.mockReturnValue(harness.runtime)

  const view = render(CanvasScreen, {
    props: {
      route,
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
        ImageToolPresentation: ImageToolPresentationStub,
        CanvasNodeContextMenu: ContextMenuStub,
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
    mocks.dragEvents.length = 0
  })

  it('renders and forwards Canvas interactions through the public Screen seam', async () => {
    const view = renderScreen()
    const { harness } = view

    expect(view.getByTestId('chrome-region')).toBeInTheDocument()
    expect(view.getByTestId('flow-region')).toBeInTheDocument()
    expect(view.getByTestId('node-region')).toBeInTheDocument()
    expect(view.getByTestId('group-region')).toBeInTheDocument()
    expect(view.getByTestId('minimap-region')).toBeInTheDocument()
    expect(view.getByTestId('image-tool-presentation')).toBeInTheDocument()
    expect(view.getByTestId('image-tool-presentation')).toHaveAttribute('data-forwarded-attrs', '')
    expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument()

    view.getByRole('button', { name: '导出选中图片' }).click()
    view.getByRole('button', { name: '缩小画布' }).click()
    view.getByRole('button', { name: '点击画布' }).click()
    view.getByRole('button', { name: '居中小地图' }).click()
    view.getByRole('button', { name: '打开节点菜单' }).click()
    await vi.waitFor(() => expect(view.getByTestId('node-context-menu')).toBeInTheDocument())
    view.getByRole('button', { name: '创建提示词节点' }).click()
    await vi.waitFor(() => expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument())

    view.getByRole('button', { name: '打开节点菜单' }).click()
    await vi.waitFor(() => expect(view.getByTestId('node-context-menu')).toBeInTheDocument())
    view.getByRole('button', { name: '触发节点交互' }).click()
    await vi.waitFor(() => expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument())

    view.getByRole('button', { name: '打开节点菜单' }).click()
    await vi.waitFor(() => expect(view.getByTestId('node-context-menu')).toBeInTheDocument())
    view.getByRole('button', { name: '删除节点' }).click()
    await vi.waitFor(() => expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument())
    view.getByRole('button', { name: '开始拖动节点' }).click()
    view.getByRole('button', { name: '结束拖动节点' }).click()
    mocks.groupOptions!.grouping.onAction()

    expect(harness.onExportSelectedImages).toHaveBeenCalledTimes(1)
    expect(harness.onZoomOut).toHaveBeenCalledTimes(1)
    expect(harness.runtime.commands.flowPane.onNodesChange).toHaveBeenCalledWith([
      { id: 'node-a', type: 'remove' },
    ])
    expect(harness.handlePaneClick).toHaveBeenCalledTimes(1)
    expect(harness.onFlowDeleteNode).toHaveBeenCalledWith('node-a')
    expect(mocks.onFlowNodeMouseDown).toHaveBeenCalledWith('node-a', expect.any(MouseEvent))
    expect(harness.runtime.commands.viewportControls.onMiniMapCenterRequested).toHaveBeenCalledWith(
      {
        position: { x: 10, y: 20 },
      },
    )
    expect(harness.runtime.commands.graphEditing.createNodeAtPosition).toHaveBeenCalledWith(
      'prompt',
      12,
      24,
    )
    expect(harness.groupSelectedNodes).toHaveBeenCalledTimes(1)
    expect(mocks.dragEvents).toEqual(['viewport:start', 'drag:start', 'drag:stop', 'viewport:stop'])
  })

  it('dismisses transient UI when group deletion succeeds through keyboard ownership', async () => {
    const view = renderScreen()
    view.harness.deleteSelectedGroup.mockReturnValueOnce(true)
    view.getByRole('button', { name: '打开节点菜单' }).click()
    await vi.waitFor(() => expect(view.getByTestId('node-context-menu')).toBeInTheDocument())

    expect(mocks.keyboardOptions!.onDeleteInlineSelection()).toBe(true)

    await vi.waitFor(() => expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument())
  })

  it('recovers the active Board session on mount', async () => {
    const { harness } = renderScreen()

    await Promise.resolve()

    expect(harness.recoverBoardSession).toHaveBeenCalledTimes(1)
  })

  it('recovers the next Board session when the route switches', async () => {
    const route = reactive({
      query: { boardId: 'board-a' },
    }) as unknown as RouteLocationNormalizedLoaded
    const { harness } = renderScreen(route)
    await vi.waitFor(() => expect(harness.recoverBoardSession).toHaveBeenCalledTimes(1))
    harness.recoverBoardSession.mockClear()

    route.query.boardId = 'board-b'

    await vi.waitFor(() => expect(harness.recoverBoardSession).toHaveBeenCalledTimes(1))
  })

  it('saves through keyboard and leave-guard behavior', async () => {
    const { harness } = renderScreen()

    mocks.keyboardOptions!.onSave()
    await mocks.leaveGuardOptions!.onLeave()

    expect(harness.flushAutosave).toHaveBeenNthCalledWith(1, { silent: false })
    expect(harness.flushAutosave).toHaveBeenNthCalledWith(2)
  })
})
