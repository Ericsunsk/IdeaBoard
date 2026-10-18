import { computed, defineComponent, h, nextTick, reactive, ref, type PropType } from 'vue'
import {
  VueFlow,
  type FlowProps,
  type GraphEdge,
  type GraphNode,
  type NodeProps,
} from '@vue-flow/core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { render } from '../../../../test/render'
import CanvasScreen from './CanvasScreen.vue'

type KeyboardOptions = {
  onSave: () => void
  onToolHand: () => void
  onToolSelect: () => void
  onDeleteInlineSelection: () => boolean
  onSpacePressedChange: (isPressed: boolean) => void
}
type LeaveGuardOptions = { onLeave: () => Promise<unknown> | unknown }
type GroupOptions = { grouping: { onAction: () => void } }

const mocks = vi.hoisted(() => ({
  keyboardOptions: null as KeyboardOptions | null,
  leaveGuardOptions: null as LeaveGuardOptions | null,
  groupOptions: null as GroupOptions | null,
  onFlowNodeMouseDown: vi.fn(),
  selectionOnPaneClick: vi.fn(),
  selectionOnFlowNodeClick: vi.fn(),
  dragEvents: [] as string[],
  useCanvasWorkspaceRuntime: vi.fn(),
  defaultEdgeOptions: undefined as FlowProps['defaultEdgeOptions'],
  flowProps: {} as Partial<FlowProps>,
  emitPaneContextMenu: (_event: MouseEvent) => {},
  groupContextMenu: vi.fn(),
}))

vi.mock('../../session/workspace', () => ({
  useCanvasWorkspaceRuntime: mocks.useCanvasWorkspaceRuntime,
}))

vi.mock('../../session/flow-adapter', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/flow-adapter')>()),
  provideCanvasViewportContext: vi.fn(),
  createCanvasConnectionDropLifecycle: () => ({
    onConnectStart: vi.fn(),
    onConnectEnd: vi.fn(),
    cancel: vi.fn(),
    dispose: vi.fn(),
  }),
  useVueFlowAdapter: () => ({
    screenToFlowCoordinate: vi.fn((point: { x: number; y: number }) => point),
    viewport: ref({ x: 0, y: 0, zoom: 1 }),
    selectOnlyNode: vi.fn(() => true),
  }),
  useCanvasVueFlowHandleGeometryAdapter: () => ({
    prepare: vi.fn(),
    resolveAnchor: vi.fn(() => null),
  }),
  useVueFlowSelectionAdapter: () => ({
    paneSelectionBindings: { onPointerDownCapture: vi.fn() },
    vueFlowSelectionBindings: {
      onEdgesChange: vi.fn(),
      onSelectionStart: vi.fn(),
      onSelectionEnd: vi.fn(),
    },
    commands: {
      clearSelection: vi.fn(() => true),
      onPaneClick: mocks.selectionOnPaneClick,
      onFlowNodeClick: mocks.selectionOnFlowNodeClick,
      onFlowNodeMouseDown: mocks.onFlowNodeMouseDown,
    },
  }),
}))

vi.mock('./useCanvasKeyboardShortcuts', () => ({
  useCanvasKeyboardShortcuts: (options: KeyboardOptions) => {
    mocks.keyboardOptions = options
  },
}))

vi.mock('./useCanvasLeaveGuard', () => ({
  useCanvasLeaveGuard: (options: LeaveGuardOptions) => {
    mocks.leaveGuardOptions = options
  },
}))

vi.mock('./useFlowNodeDragInteractions', () => ({
  useFlowNodeDragInteractions: () => ({
    onNodeDragStart: () => mocks.dragEvents.push('drag:start'),
    onNodeDragStop: () => mocks.dragEvents.push('drag:stop'),
  }),
}))

vi.mock('./useCanvasImageLightbox', () => ({
  useCanvasImageLightbox: () => ({
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
  }),
}))

vi.mock('../nodes/groups', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../nodes/groups')>()
  return {
    ...actual,
    useCanvasGroupPresentationInteraction: (options: GroupOptions) => {
      mocks.groupOptions = options
      return {
        bindClosePolicyHandlers: <T extends Record<string, unknown>>(handlers: T) => ({
          ...handlers,
          onCanvasPaneClick: handlers.onCanvasPaneClick,
        }),
        onCanvasContextMenuCapture: mocks.groupContextMenu,
        onCanvasPointerDownCapture: vi.fn(),
      }
    },
  }
})

vi.mock('../overlays', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../overlays')>()),
  useCanvasCustomLayerOverlay: vi.fn(),
}))

vi.mock('../../../workspace/shell/useWorkspaceAccountControls.ts', () => ({
  useWorkspaceAccountControls: () => ({
    viewModel: { avatarUrl: ref(null), userEmail: ref(null), balance: ref({}), menuItems: ref([]) },
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
  setup(_, { emit, slots, attrs }) {
    mocks.emitPaneContextMenu = (event) => emit('pane-context-menu', event)
    mocks.defaultEdgeOptions = attrs.defaultEdgeOptions as FlowProps['defaultEdgeOptions']
    return () => {
      mocks.flowProps = attrs as Partial<FlowProps>
      return h('section', { 'data-testid': 'flow-region', class: 'vue-flow__pane' }, [
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
            onClick: () => {
              const event = new MouseEvent('mouseup', { clientX: 12, clientY: 24 })
              document.querySelector('[data-testid="flow-region"]')?.dispatchEvent(event)
              event.stopPropagation()
              emit('pane-context-menu', event)
            },
          },
          '打开节点菜单',
        ),
        h(
          'button',
          {
            type: 'button',
            onClick: () => emit('node-drag-start', { nodes: [{ id: 'node-a' }] }),
          },
          '开始拖动节点',
        ),
        h('button', { type: 'button', onClick: () => emit('node-drag-stop') }, '结束拖动节点'),
        slots['zoom-pane']?.(),
        slots['node-canvas']?.({
          id: 'node-a',
          data: {},
          selected: false,
          dragging: false,
        } satisfies Partial<NodeProps>),
        h('div', { class: 'vue-flow__edge', 'data-testid': 'edge-region' }),
        h('div', {
          class: 'vue-flow__nodesselection-rect',
          'data-testid': 'selected-overlay-region',
        }),
        slots.default?.(),
      ])
    }
  },
})

const GroupStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'group-region', class: 'canvas-group-frame-body' })
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
          class: 'vue-flow__node',
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
    isVisible: { type: Boolean, required: true },
    x: { type: Number, required: true },
    y: { type: Number, required: true },
    items: { type: Array as PropType<Array<{ key: string }>>, required: true },
  },
  emits: ['create'],
  setup(props, { emit }) {
    return () =>
      props.isVisible
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

function createRuntimeViewModel() {
  const selectionGroupAction = ref({ kind: 'group' as const })
  const imageToolsViewModel = {
    active: { nodeId: ref(null), maskNodeId: ref(null) },
    locks: { isChromeHidden: ref(false), isCanvasLocked: ref(false) },
    retouch: { isApplying: ref(false) },
    nodeBindings: {
      stageFor: vi.fn(() => null),
      actionFor: vi.fn((nodeId: string) => ({ nodeId })),
    },
    chrome: ref({ isReady: false }),
  }
  return {
    flowPane: { nodes: ref([]), edges: ref([]) },
    boardChrome: {
      currentBoardId: ref('board-a'),
      isBoardMenuOpen: ref(false),
      isRenamingBoard: ref(false),
      boardNameDraft: ref('Board A'),
      isRenamePending: ref(false),
      catalogRecovery: ref({ status: 'idle' }),
      isBoardArchived: ref(false),
      isBoardReadOnly: ref(false),
      isRestoringArchivedBoard: ref(false),
      currentBoardName: ref('Board A'),
      boardOptions: ref([]),
    },
    persistence: { history: { undo: vi.fn(), redo: vi.fn() } },
    viewportControls: {
      zoomLevel: ref(100),
      imageQuality: { zoomLevel: ref(100), isSettled: ref(true), requestSettle: vi.fn() },
    },
    graphEditing: {
      hasSelectedFlowNodes: ref(false),
      isFlowInteractionActive: ref(false),
      isFlowViewportPanInteractionActive: ref(false),
      isFlowInteractionLocked: ref(false),
      grouping: { groupFrames: ref([]), selectionGroupAction },
    },
    imageWorkflow: {
      tools: imageToolsViewModel,
      outpaint: { nodeId: ref(null), overlayStyle: ref(null), handleRadius: 4 },
    },
  }
}

function createImageWorkflowCommands() {
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
    onFlowImageOutpaintSubmit: vi.fn(),
    onFlowOutpaintHandlePointerDown: vi.fn(),
    onFlowOutpaintFramePointerDown: vi.fn(),
    onFlowOutpaintHandlePointerMove: vi.fn(),
    onFlowOutpaintHandlePointerUp: vi.fn(),
  }

  return {
    isGenerationAvailable: vi.fn(() => true),
    tools: imageToolCommands,
    interactions: imageWorkflowInteractions,
    cancelOutpaint: vi.fn(),
    hideStrengthControl: vi.fn(),
  }
}

function createRuntime() {
  const flushAutosave = vi.fn()
  const recoverBoardSession = vi.fn(async () => {})
  const onZoomOut = vi.fn()
  const onExportSelectedImages = vi.fn()
  const handlePaneClick = vi.fn()
  const groupSelectedNodes = vi.fn()
  const deleteSelectedGroup = vi.fn(() => false)
  const onFlowDeleteNode = vi.fn()

  return {
    runtime: {
      viewModel: createRuntimeViewModel(),
      commands: {
        boardSession: {
          setBoardNameInputRef: vi.fn(),
          toggleBoardMenu: vi.fn(),
          startBoardRename: vi.fn(),
          cancelBoardRename: vi.fn(),
          closeBoardHeaderEditing: vi.fn(),
          onConfirmBoardRename: vi.fn(),
          reloadCatalog: vi.fn(),
          restoreArchivedBoard: vi.fn(),
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
            dismissTransientCanvasUi: (options: { shouldCloseContextMenu?: boolean }) => void
          }) => ({
            onFlowDeleteNode: (nodeId: string) => {
              interactionOptions.dismissTransientCanvasUi({ shouldCloseContextMenu: true })
              onFlowDeleteNode(nodeId)
            },
            onFlowPromptTextUpdate: vi.fn(),
            onFlowNodeParamUpdate: vi.fn(),
            onFlowImageOpenFile: vi.fn(),
            onFlowImageReset: vi.fn(),
            onFlowImageStageDrop: vi.fn(),
          }),
        },
        imageWorkflow: createImageWorkflowCommands(),
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
  isNativeFlow = false,
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
      plugins: [[VueQueryPlugin, { queryClient: new QueryClient() }]],
      stubs: {
        CanvasChrome: ChromeStub,
        VueFlow: isNativeFlow ? VueFlow : VueFlowStub,
        CanvasGroupPresentation: GroupStub,
        FlowCanvasNode: NodeStub,
        CanvasChromeMiniMap: MiniMapStub,
        ImageToolPresentation: ImageToolPresentationStub,
        Presentation: ImageToolPresentationStub,
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
    mocks.defaultEdgeOptions = undefined
    mocks.flowProps = {}
  })

  it('uses direct trackpad navigation and middle/right panning in select mode', () => {
    renderScreen()
    expect(mocks.flowProps).toMatchObject({
      panOnDrag: [1, 2],
      panOnScroll: true,
      panOnScrollSpeed: 1.2,
      autoPanSpeed: 15,
      zoomOnScroll: false,
      zoomOnPinch: true,
      zoomOnDoubleClick: false,
      minZoom: 0.15,
      maxZoom: 2,
      nodesDraggable: true,
      elementsSelectable: true,
    } satisfies Partial<FlowProps>)
  })

  it('enables all mouse buttons in hand mode while disabling graph editing', async () => {
    renderScreen()
    mocks.keyboardOptions!.onToolHand()
    await nextTick()
    expect(mocks.flowProps).toMatchObject({
      panOnDrag: [0, 1, 2],
      nodesDraggable: false,
      nodesConnectable: false,
      elementsSelectable: false,
      selectionKeyCode: false,
    } satisfies Partial<FlowProps>)
    mocks.keyboardOptions!.onToolSelect()
    await nextTick()
    expect(mocks.flowProps.panOnDrag).toEqual([1, 2])
  })

  it.each(['locked', 'archived', 'read-only'] as const)(
    'disables navigation for %s boards',
    async (state) => {
      const view = renderScreen()
      if (state === 'archived')
        view.harness.runtime.viewModel.boardChrome.isBoardArchived.value = true
      else if (state === 'read-only')
        view.harness.runtime.viewModel.boardChrome.isBoardReadOnly.value = true
      else view.harness.runtime.viewModel.imageWorkflow.tools.locks.isCanvasLocked.value = true
      await nextTick()
      expect(mocks.flowProps).toMatchObject({
        panOnDrag: false,
        panOnScroll: false,
        zoomOnPinch: false,
        zoomOnDoubleClick: false,
        nodesDraggable: false,
        elementsSelectable: false,
      } satisfies Partial<FlowProps>)
    },
  )

  it('uses temporary Space Hand mode and restores the selected tool on release', async () => {
    renderScreen()
    mocks.keyboardOptions!.onSpacePressedChange(true)
    await nextTick()
    expect(mocks.flowProps).toMatchObject({
      panOnDrag: [0, 1, 2],
      nodesDraggable: false,
      selectionKeyCode: false,
      panActivationKeyCode: null,
    } satisfies Partial<FlowProps>)
    mocks.keyboardOptions!.onSpacePressedChange(false)
    await nextTick()
    expect(mocks.flowProps.panOnDrag).toEqual([1, 2])
    mocks.keyboardOptions!.onToolHand()
    mocks.keyboardOptions!.onSpacePressedChange(true)
    mocks.keyboardOptions!.onSpacePressedChange(false)
    await nextTick()
    expect(mocks.flowProps.panOnDrag).toEqual([0, 1, 2])
  })

  describe.each(['pane', 'group'] as const)('native %s pan gestures', (target) => {
    afterEach(async () => {
      // D3 releases post-drag click suppression on the next task.
      await new Promise<void>((resolve) => setTimeout(resolve, 0))
    })
    function nativeHarness() {
      const view = renderScreen(undefined, true)
      const element =
        target === 'pane'
          ? view.container.getElementsByClassName('vue-flow__pane')[0]!
          : view.getByTestId('group-region')
      const mouse = (type: string, x = 20, y = 30, init: MouseEventInit = {}) => {
        const event = new MouseEvent(type, {
          bubbles: true,
          cancelable: true,
          button: 2,
          buttons: type === 'mouseup' ? 0 : 2,
          clientX: x,
          clientY: y,
          view: window,
          ...init,
        })
        element.dispatchEvent(event)
        return event
      }
      return { ...view, mouse }
    }

    it.each(['press', 'release'])(
      'opens one settled menu across %s-time contextmenu ordering',
      async (timing) => {
        const view = nativeHarness()
        await nextTick()
        view.mouse('mousedown')
        if (timing === 'press') view.mouse('contextmenu')
        expect(mocks.groupContextMenu).not.toHaveBeenCalled()
        view.mouse('mouseup')
        if (timing === 'release') view.mouse('contextmenu')
        await nextTick()
        expect(mocks.groupContextMenu).toHaveBeenCalledTimes(1)
        expect(mocks.groupContextMenu.mock.calls[0]?.[0].type).toBe('mouseup')
        expect(view.getByTestId('node-context-menu')).toBeInTheDocument()
      },
    )

    it.each(['press', 'release'])(
      'does not open a menu after right panning with %s-time contextmenu',
      async (timing) => {
        const view = nativeHarness()
        await nextTick()
        view.mouse('mousedown')
        if (timing === 'press') view.mouse('contextmenu')
        view.mouse('mousemove', 60, 70)
        view.mouse('mousemove')
        view.mouse('mouseup')
        if (timing === 'release') view.mouse('contextmenu')
        await nextTick()
        expect(mocks.groupContextMenu).not.toHaveBeenCalled()
        expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument()
      },
    )

    it('pans with temporary Space Hand while Ctrl+Space prioritizes context', async () => {
      const view = nativeHarness()
      await nextTick()
      mocks.keyboardOptions!.onSpacePressedChange(true)
      await nextTick()
      view.mouse('mousedown', 20, 30, { button: 0, buttons: 1 })
      view.mouse('mousemove', 60, 70, { button: 0, buttons: 1 })
      view.mouse('mouseup', 60, 70, { button: 0 })
      await nextTick()
      const onMove = view.harness.runtime.commands.viewportControls.onFlowMove
      expect(onMove).toHaveBeenCalled()
      onMove.mockClear()

      view.mouse('mousedown', 20, 30, { ctrlKey: true })
      view.mouse('contextmenu', 20, 30, { ctrlKey: true })
      view.mouse('mousemove', 60, 70, { ctrlKey: true })
      view.mouse('mouseup', 60, 70, { ctrlKey: true })
      await nextTick()
      expect(onMove).not.toHaveBeenCalled()
      expect(mocks.groupContextMenu).toHaveBeenCalledTimes(1)
      expect(view.getByTestId('node-context-menu')).toBeInTheDocument()
      mocks.keyboardOptions!.onSpacePressedChange(false)
    })
  })

  it('lets a selected group handle the settled callback without opening the creation menu', async () => {
    const view = renderScreen()
    const event = new MouseEvent('mouseup', { button: 2 })
    view.getByTestId('flow-region').dispatchEvent(event)
    event.stopPropagation()
    mocks.groupContextMenu.mockReturnValueOnce(true)
    mocks.emitPaneContextMenu(event)
    await nextTick()
    expect(mocks.groupContextMenu).toHaveBeenCalledWith(event)
    expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument()
  })

  it.each(['node-region', 'edge-region', 'selected-overlay-region'])(
    'preserves native %s context routing and ignores its pan callback',
    async (region) => {
      const view = renderScreen()
      const target = view.getByTestId(region)
      const contextMenu = new MouseEvent('contextmenu', { bubbles: true, button: 2 })
      target.dispatchEvent(contextMenu)
      expect(mocks.groupContextMenu).toHaveBeenCalledExactlyOnceWith(contextMenu)
      const event = new MouseEvent('mouseup', { button: 2 })
      target.dispatchEvent(event)
      mocks.emitPaneContextMenu(event)
      await nextTick()
      expect(mocks.groupContextMenu).toHaveBeenCalledTimes(1)
      expect(view.queryByTestId('node-context-menu')).not.toBeInTheDocument()
    },
  )

  it('opens keyboard context menus without waiting for a pointer gesture', async () => {
    const view = renderScreen()
    const target = view.getByTestId('flow-region')
    target.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, button: 2 }))
    target.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, button: 0 }))
    await nextTick()
    expect(mocks.groupContextMenu).toHaveBeenCalledTimes(1)
    expect(view.getByTestId('node-context-menu')).toBeInTheDocument()
  })

  it.each([0, 2])('prioritizes Ctrl context over background button %i panning', async (button) => {
    const view = renderScreen()
    const target = view.getByTestId('flow-region')
    const onMouseDown = vi.fn()
    target.addEventListener('mousedown', onMouseDown)
    const mouseDown = new MouseEvent('mousedown', {
      bubbles: true,
      cancelable: true,
      button,
      ctrlKey: true,
    })
    target.dispatchEvent(mouseDown)
    expect(mouseDown.defaultPrevented).toBe(true)
    expect(onMouseDown).not.toHaveBeenCalled()
    target.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, button: 2, ctrlKey: true }))
    await nextTick()
    expect(mocks.groupContextMenu).toHaveBeenCalledTimes(1)
    expect(view.getByTestId('node-context-menu')).toBeInTheDocument()
  })

  it.each(['locked', 'archived'] as const)(
    'blocks upstream middle-button graph navigation when %s',
    async (state) => {
      const view = renderScreen()
      if (state === 'locked')
        view.harness.runtime.viewModel.imageWorkflow.tools.locks.isCanvasLocked.value = true
      else view.harness.runtime.viewModel.boardChrome.isBoardArchived.value = true
      await nextTick()
      const target = view.getByTestId('node-region')
      const event = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 1 })
      const onMouseDown = vi.fn()
      target.addEventListener('mousedown', onMouseDown)
      target.dispatchEvent(event)
      expect(event.defaultPrevented).toBe(true)
      expect(onMouseDown).not.toHaveBeenCalled()
    },
  )

  it('updates the shared node UI scale without changing image quality during zoom', async () => {
    const view = renderScreen()
    const surface = view.container.querySelector<HTMLElement>('.canvas-flow-wrap')!
    expect(surface.style.getPropertyValue('--canvas-node-ui-scale')).toBe('1')

    view.harness.runtime.viewModel.viewportControls.zoomLevel.value = 50
    await nextTick()

    expect(surface.style.getPropertyValue('--canvas-node-ui-scale')).toBe('2')
    expect(view.harness.runtime.viewModel.viewportControls.imageQuality.zoomLevel.value).toBe(100)
  })

  it('highlights outgoing edges as their source selection changes without selecting the edges', () => {
    renderScreen()
    const edgeClass = mocks.defaultEdgeOptions?.class
    if (typeof edgeClass !== 'function') throw new Error('expected an edge class callback')
    const source = reactive<Pick<GraphNode, 'id' | 'selected'>>({ id: 'node-a', selected: false })
    const target = reactive<Pick<GraphNode, 'id' | 'selected'>>({ id: 'node-b', selected: false })
    const outgoing = {
      id: 'edge-a-b',
      source: source.id,
      target: target.id,
      sourceNode: source,
      targetNode: target,
      selected: false,
    } as GraphEdge
    const incoming = {
      ...outgoing,
      id: 'edge-b-a',
      source: target.id,
      target: source.id,
      sourceNode: target,
      targetNode: source,
    } as GraphEdge
    const outgoingClass = computed(() => edgeClass(outgoing))
    const incomingClass = computed(() => edgeClass(incoming))

    expect(outgoingClass.value).toBeUndefined()
    source.selected = true
    expect(outgoingClass.value).toBe('is-source-selected')
    expect(incomingClass.value).toBeUndefined()
    target.selected = true
    expect(incomingClass.value).toBe('is-source-selected')
    source.selected = false
    expect(outgoingClass.value).toBeUndefined()
    expect(incomingClass.value).toBe('is-source-selected')
    target.selected = false
    expect(incomingClass.value).toBeUndefined()
    expect(outgoing.selected).toBe(false)
    expect(incoming.selected).toBe(false)
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
    expect(mocks.selectionOnPaneClick).toHaveBeenCalledTimes(1)
    // Pane clicks must run group intent before pane selection.
    expect(harness.handlePaneClick.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.selectionOnPaneClick.mock.invocationCallOrder[0]!,
    )
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

    expect(harness.flushAutosave).toHaveBeenNthCalledWith(1, { isSilent: false })
    expect(harness.flushAutosave).toHaveBeenNthCalledWith(2)
  })
})
