import '@testing-library/jest-dom/vitest'

// Canvas immutable connection endpoints browser contract.
//
// Observable replacement for the retired
// `canvasScreenArchitecture.contract.spec.ts` "keeps connection endpoints
// immutable in the Vue Flow surface" source-string assertions (lines 56-60,
// `behavior-existing-partial`, 5 IDs): every retired constraint needs an
// effective behavior replacement passing first. Follows the board/search P3
// precedent: assert rendered behavior — never file text.
//
// Coverage map (ledger
// `src/features/canvas/presentation/screens/canvasScreenArchitecture.contract.spec.ts`):
// - `:edges="renderEdges"` (L56): the rendered edge set tracks the
//   projected renderEdges ref live (add/remove propagates to the DOM).
// - `onlyRenderVisibleElements: false` (L57): an off-viewport node still
//   renders its VueFlow wrapper (no virtualization culling).
// - `edgesUpdatable: false` (L58): no edge-update handles render.
// - `connectOnClick: false` (L59): clicking two nodes creates no edge.
// - `defaultEdgeOptions: { animated: false, updatable: false }` (L60): no
//   edge animates.
//
// The mount reuses the `CanvasScreen.component.spec.ts` runtime harness
// with the real VueFlow component (only the node chrome stays stubbed):
// the options under test reach VueFlow through the Screen's computed
// props, so flipping any of them in `CanvasScreen.vue` fails below.

import { page } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref, type PropType } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { VueFlow } from '@vue-flow/core'
import { render } from '../../../../test/render'
import CanvasScreen from './CanvasScreen.vue'
import '../../../../app/style.css'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

const mocks = vi.hoisted(() => ({
  keyboardOptions: null as { onSave: () => void; onDeleteInlineSelection: () => boolean } | null,
  leaveGuardOptions: null as { onLeave: () => Promise<unknown> | unknown } | null,
  groupOptions: null as { grouping: { onAction: () => void } } | null,
  onFlowNodeMouseDown: vi.fn(),
  selectionOnPaneClick: vi.fn(),
  selectionOnFlowNodeClick: vi.fn(),
  dragEvents: [] as string[],
  useCanvasWorkspaceRuntime: vi.fn(),
}))

vi.mock('../../session/workspace', () => ({
  useCanvasWorkspaceRuntime: mocks.useCanvasWorkspaceRuntime,
}))

vi.mock('../../session/flow-adapter', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../session/flow-adapter')>()
  return {
    ...actual,
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
  }
})

vi.mock('./useCanvasKeyboardShortcuts', () => ({
  useCanvasKeyboardShortcuts: (options: NonNullable<typeof mocks.keyboardOptions>) => {
    mocks.keyboardOptions = options
  },
}))

vi.mock('./useCanvasLeaveGuard', () => ({
  useCanvasLeaveGuard: (options: NonNullable<typeof mocks.leaveGuardOptions>) => {
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
    useCanvasGroupPresentationInteraction: (options: NonNullable<typeof mocks.groupOptions>) => {
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

vi.mock('../overlays', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../overlays')>()
  return { ...actual, useCanvasCustomLayerOverlay: vi.fn() }
})

vi.mock('../../../workspace/shell/useWorkspaceAccountControls.ts', () => ({
  useWorkspaceAccountControls: () => ({
    viewModel: { avatarUrl: ref(null), userEmail: ref(null), balance: ref({}), menuItems: ref([]) },
    commands: { openAccount: vi.fn() },
  }),
}))

const ChromeStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'chrome-region' })
  },
})

const GroupStub = defineComponent({
  setup() {
    return () => h('section', { 'data-testid': 'group-region' })
  },
})

const NodeStub = defineComponent({
  setup() {
    return () => h('div', { 'data-testid': 'node-stub' }, 'node')
  },
})

const MiniMapStub = defineComponent({
  setup() {
    return () => h('div', { 'data-testid': 'minimap-region' })
  },
})

const ImageToolPresentationStub = defineComponent({
  props: { imageTools: { type: Object, required: true } },
  setup() {
    return () => h('section', { 'data-testid': 'image-tool-presentation' })
  },
})

const ContextMenuStub = defineComponent({
  props: {
    isVisible: { type: Boolean, required: true },
    x: { type: Number, required: true },
    y: { type: Number, required: true },
    items: { type: Array as PropType<Array<{ key: string }>>, required: true },
  },
  setup() {
    return () => null
  },
})

function createRuntimeFlow(flowNodes: { id: string; position: { x: number; y: number } }[]) {
  const nodes = ref(
    flowNodes.map((node) => ({ ...node, style: { width: '120px', height: '60px' } })),
  )
  const renderEdges = ref([{ id: 'edge-a-b', source: 'node-a', target: 'node-b' }])
  return { nodes, renderEdges }
}

function createRuntimeViewModel({ nodes, renderEdges }: ReturnType<typeof createRuntimeFlow>) {
  return {
    flowPane: { nodes, edges: renderEdges, renderEdges },
    boardChrome: {
      currentBoardId: ref('board-a'),
      isBoardMenuOpen: ref(false),
      isRenamingBoard: ref(false),
      boardNameDraft: ref('Board A'),
      isRenamePending: ref(false),
      catalogRecovery: ref({ status: 'idle' }),
      isBoardArchived: ref(false),
      isRestoringArchivedBoard: ref(false),
      currentBoardName: ref('Board A'),
      boardOptions: ref([]),
    },
    persistence: { history: { undo: vi.fn(), redo: vi.fn() } },
    viewportControls: { zoomLevel: ref(100), qualityZoomLevel: ref(100) },
    graphEditing: {
      hasSelectedFlowNodes: ref(false),
      isFlowInteractionActive: ref(false),
      isFlowViewportPanInteractionActive: ref(false),
      isFlowInteractionLocked: ref(false),
      grouping: { groupFrames: ref([]), selectionGroupAction: ref({ kind: 'group' as const }) },
    },
    imageWorkflow: {
      tools: {
        active: { nodeId: ref(null), maskNodeId: ref(null) },
        locks: { isChromeHidden: ref(false), isCanvasLocked: ref(false) },
        nodeBindings: { stageFor: vi.fn(() => null), actionFor: vi.fn() },
        chrome: ref({ isReady: false }),
      },
      outpaint: { nodeId: ref(null), overlayStyle: ref(null), handleRadius: 4 },
    },
  }
}

function createRuntimeCommands() {
  const flushAutosave = vi.fn()
  const recoverBoardSession = vi.fn(async () => {})
  return {
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
    persistence: { flushAutosave, recoverBoardSession },
    viewportControls: {
      onZoomIn: vi.fn(),
      onZoomOut: vi.fn(),
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
      libraryImport: { add: vi.fn() },
      imageImport: { retry: vi.fn(async () => false) },
      grouping: {
        selectGroupAtPoint: vi.fn(),
        handlePaneClick: vi.fn(),
        handleNodeClick: vi.fn(),
        groupSelectedNodes: vi.fn(),
        ungroupSelectedGroup: vi.fn(),
        deleteSelectedGroup: vi.fn(() => false),
        groupDrag: vi.fn(),
        groupResize: vi.fn(),
      },
      createNodeAtPosition: vi.fn(),
      onExportSelectedImages: vi.fn(),
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
      tools: {
        lifecycle: { enter: vi.fn(), cancel: vi.fn(), discardNodeEdits: vi.fn() },
        editor: {
          onCanvasWheel: vi.fn(),
          focusAndShowChrome: vi.fn(),
          scheduleChromeAnchorUpdate: vi.fn(),
        },
        chrome: {},
      },
      interactions: {
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
      },
      cancelOutpaint: vi.fn(),
      hideStrengthControl: vi.fn(),
    },
  }
}

function createRuntime(flowNodes: { id: string; position: { x: number; y: number } }[]) {
  const flow = createRuntimeFlow(flowNodes)
  mocks.useCanvasWorkspaceRuntime.mockReturnValue({
    viewModel: createRuntimeViewModel(flow),
    commands: createRuntimeCommands(),
  })
  return flow
}

function renderScreen() {
  const flow = createRuntime([
    { id: 'node-a', position: { x: 50, y: 60 } },
    { id: 'node-b', position: { x: 400, y: 60 } },
    { id: 'node-far', position: { x: 5000, y: 5000 } },
  ])
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
        VueFlow,
        CanvasGroupPresentation: GroupStub,
        FlowCanvasNode: NodeStub,
        CanvasChromeMiniMap: MiniMapStub,
        ImageToolPresentation: ImageToolPresentationStub,
        Presentation: ImageToolPresentationStub,
        CanvasNodeContextMenu: ContextMenuStub,
      },
    },
  })
  return { flow, ...view }
}

async function waitFor<T>(read: () => T | null, label: string): Promise<T> {
  const started = Date.now()
  for (;;) {
    const value = read()
    if (value) return value
    if (Date.now() - started > 8000) throw new Error(`${label} missing`)
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
}

describe('Canvas immutable connection endpoints browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1400, 800)
    vi.clearAllMocks()
    mocks.keyboardOptions = null
    mocks.leaveGuardOptions = null
    mocks.groupOptions = null
    mocks.dragEvents.length = 0
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('binds the rendered edges to the projected renderEdges ref', async () => {
    const { flow, container, unmount } = renderScreen()
    try {
      await waitFor(() => container.querySelector('.vue-flow__edge'), 'edge')
      expect(container.querySelectorAll('.vue-flow__edge')).toHaveLength(1)

      flow.renderEdges.value = [
        { id: 'edge-a-b', source: 'node-a', target: 'node-b' },
        { id: 'edge-b-far', source: 'node-b', target: 'node-far' },
      ]
      await nextTick()
      await waitFor(
        () =>
          container.querySelectorAll('.vue-flow__edge').length === 2
            ? container.querySelector('.vue-flow__edge')
            : null,
        'second edge',
      )
      expect(container.querySelectorAll('.vue-flow__edge')).toHaveLength(2)

      flow.renderEdges.value = []
      await nextTick()
      await waitFor(
        () =>
          container.querySelectorAll('.vue-flow__edge').length === 0
            ? (container.querySelector('.vue-flow') as Element)
            : null,
        'edge removal',
      )
      expect(container.querySelectorAll('.vue-flow__edge')).toHaveLength(0)
    } finally {
      unmount()
    }
  })

  it('renders off-viewport nodes without virtualization culling', async () => {
    const { container, unmount } = renderScreen()
    try {
      await waitFor(() => container.querySelector('.vue-flow__node'), 'node')
      const renderedIds = [...container.querySelectorAll('.vue-flow__node')]
        .map((node) => node.getAttribute('data-id'))
        .sort()
      // node-far sits at (5000, 5000), far outside the 1400x800 viewport:
      // with onlyRenderVisibleElements: false it still renders.
      expect(renderedIds).toEqual(['node-a', 'node-b', 'node-far'])
    } finally {
      unmount()
    }
  })

  it('renders no edge-update handles and creates no edge on node click', async () => {
    const { container, unmount } = renderScreen()
    try {
      await waitFor(() => container.querySelector('.vue-flow__edge'), 'edge')
      // edgesUpdatable: false — VueFlow renders edge-update anchors only
      // for updatable edges.
      expect(container.querySelectorAll('.vue-flow__edgeupdater')).toHaveLength(0)

      // connectOnClick: false — clicking two nodes must not connect them.
      // (Dispatched at the wrapper center: the zero-chrome stub nodes
      // defeat pointer actionability checks, but the click traverses
      // VueFlow's real node-click/connect guard path.)
      const nodeA = await waitFor(
        () => container.querySelector<HTMLElement>('.vue-flow__node[data-id="node-a"]'),
        'node-a',
      )
      const nodeB = container.querySelector<HTMLElement>('.vue-flow__node[data-id="node-b"]')!
      for (const target of [nodeA, nodeB]) {
        const rect = target.getBoundingClientRect()
        target.dispatchEvent(
          new MouseEvent('click', {
            bubbles: true,
            cancelable: true,
            clientX: rect.left + rect.width / 2,
            clientY: rect.top + rect.height / 2,
          }),
        )
      }
      await nextTick()
      await new Promise((resolve) => setTimeout(resolve, 300))
      expect(container.querySelectorAll('.vue-flow__edge')).toHaveLength(1)
    } finally {
      unmount()
    }
  })

  it('animates no edge by default', async () => {
    const { container, unmount } = renderScreen()
    try {
      const edge = await waitFor(() => container.querySelector('.vue-flow__edge'), 'edge')
      expect(edge.classList.contains('animated')).toBe(false)
      const path = container.querySelector<SVGElement>('.vue-flow__edge-path')!
      expect(window.getComputedStyle(path).animationName).toBe('none')
    } finally {
      unmount()
    }
  })

  it('forwards the immutable endpoint options to the flow surface', async () => {
    let captured: Record<string, unknown> | null = null
    const PropsProbe = defineComponent({
      setup(_, { attrs }) {
        captured = { ...attrs }
        return () => null
      },
    })
    const flow = createRuntime([
      { id: 'node-a', position: { x: 50, y: 60 } },
      { id: 'node-b', position: { x: 400, y: 60 } },
    ])
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
          VueFlow: PropsProbe,
          CanvasGroupPresentation: GroupStub,
          FlowCanvasNode: NodeStub,
          CanvasChromeMiniMap: MiniMapStub,
          ImageToolPresentation: ImageToolPresentationStub,
          Presentation: ImageToolPresentationStub,
          CanvasNodeContextMenu: ContextMenuStub,
        },
      },
    })
    try {
      await waitFor(() => (captured ? true : null), 'flow props')
      const props = captured as unknown as Record<string, unknown>
      // The rendered edge set IS the projected ref (binding identity, not
      // a snapshot copy).
      expect(props['edges']).toBe(flow.renderEdges.value)
      expect(props['onlyRenderVisibleElements']).toBe(false)
      expect(props['edgesUpdatable']).toBe(false)
      expect(props['connectOnClick']).toBe(false)
      expect(props['defaultEdgeOptions']).toMatchObject({
        animated: false,
        updatable: false,
      } satisfies Partial<import('@vue-flow/core').Edge>)
    } finally {
      view.unmount()
    }
  })
})
