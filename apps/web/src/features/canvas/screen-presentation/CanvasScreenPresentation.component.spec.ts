/* eslint-disable vue/one-component-per-file */
import { defineComponent, h, ref, type PropType } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render } from '../../../test/render'
import CanvasScreenPresentation from './CanvasScreenPresentation.vue'

function createPresentation(
  overrides: {
    chromeHidden?: boolean
    exportDisabled?: boolean
    generationAvailable?: boolean
  } = {},
) {
  const nodes = ref([])
  const edges = ref([])

  return {
    layout: {
      activeTool: 'select' as const,
      isInteractionActive: false,
      isViewportPanActive: false,
      onBackgroundMouseDown: vi.fn(),
    },
    chrome: {
      props: {
        inert: overrides.chromeHidden ?? false,
        isHidden: overrides.chromeHidden ?? false,
        board: {},
        account: {},
        tools: {},
        status: { exportDisabled: overrides.exportDisabled ?? false },
      },
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
      nodes,
      edges,
      props: {},
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
      group: { props: {} },
      node: {
        propsFor: () => ({ generationAvailable: overrides.generationAvailable ?? true }),
      },
      minimap: {
        props: { enabled: true, inert: false, isHidden: false },
        commands: {
          centerRequested: vi.fn(),
          dragStart: vi.fn(),
          dragMove: vi.fn(),
          dragEnd: vi.fn(),
        },
      },
      overlays: { props: {}, commands: { createNode: vi.fn() } },
    },
  }
}

const VueFlowStub = defineComponent({
  emits: ['nodes-change'],
  setup(_, { emit, slots }) {
    return () =>
      h('section', { 'data-testid': 'flow-region' }, [
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
      ])
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
  setup(props) {
    return () =>
      h('section', {
        'data-testid': 'node-region',
        'data-generation-available': `${props.generationAvailable}`,
      })
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
  const presentation = createPresentation(input)
  const view = render(CanvasScreenPresentation, {
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

  it('projects hidden Chrome and disabled generation/export states into their regions', () => {
    const view = renderPresentation({
      chromeHidden: true,
      exportDisabled: true,
      generationAvailable: false,
    })

    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-hidden', 'true')
    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-inert', 'true')
    expect(view.getByTestId('chrome-region')).toHaveAttribute('data-export-disabled', 'true')
    expect(view.getByTestId('node-region')).toHaveAttribute('data-generation-available', 'false')
  })

  it('forwards Vue Flow node changes to the semantic flow command', () => {
    const view = renderPresentation()

    view.getByRole('button', { name: '触发节点变更' }).click()

    expect(view.presentation.canvas.flow.onNodesChange).toHaveBeenCalledWith([
      { id: 'node-1', type: 'remove' },
    ])
  })
})
