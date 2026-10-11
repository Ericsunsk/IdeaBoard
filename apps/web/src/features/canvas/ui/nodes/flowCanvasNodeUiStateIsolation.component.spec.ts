import { computed, defineComponent, h, nextTick, onUpdated, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render } from '../../../../test/render'
import FlowCanvasNode from './FlowCanvasNode.vue'
import { provideCanvasNodeUiStateContext } from '../../session/flow-adapter/handles/useCanvasNodeUiStateContext'
import type { NodeUiState } from '../../core/nodes/canvasNodeUi.ts'
import type { FusionNodeItem } from '../../../board'
import type { FlowNodeData } from '../../core/nodes/flowTypes.ts'
import { createImageNodePresentationResolver } from '.'

type MarkedUiState = NodeUiState & { marker: string }

function markedUiState(marker: string): MarkedUiState {
  return {
    connection: { hasAnyConnection: false, connected: {} },
    marker,
  }
}

const fusionRenderCounts = new Map<string, number>()

const FusionProbe = defineComponent({
  props: {
    node: { type: Object, default: undefined },
    uiState: { type: Object, default: undefined },
  },
  setup(props) {
    const title = computed(() => (props.node as { title?: string } | undefined)?.title ?? 'unknown')
    fusionRenderCounts.set(title.value, (fusionRenderCounts.get(title.value) ?? 0) + 1)
    onUpdated(() => {
      fusionRenderCounts.set(title.value, (fusionRenderCounts.get(title.value) ?? 1) + 1)
    })
    return () =>
      h(
        'div',
        { 'data-testid': `fusion-probe-${title.value}` },
        JSON.stringify((props.uiState as MarkedUiState | undefined)?.marker ?? null),
      )
  },
})

function createFusionSnapshot(title: string): FusionNodeItem {
  return {
    id: title === 'Fusion One' ? 1 : 2,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 280,
    title,
    prompt: '',
    outputCount: 1,
    aspectRatio: 'auto',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}

function createFusionData(snapshot: FusionNodeItem): FlowNodeData {
  return {
    kind: 'fusion',
    nodeType: 'fusion',
    title: snapshot.title ?? 'Fusion',
    snapshotNode: snapshot,
  }
}

function createImagePresentation(nodeId: string) {
  return createImageNodePresentationResolver({
    actionBindingFor: () => ({
      actionMenuVisible: false,
      promptComposerVisible: false,
      promptPlaceholder: '',
      directGenerating: false,
      strengthControl: {
        visible: false,
        activeAction: null,
        getValue: () => 50,
        setRangeRef: vi.fn(),
        setTriggerRef: vi.fn(),
        onRangePointerDown: vi.fn(),
        onInput: vi.fn(),
        onChange: vi.fn(),
        onPointerUp: vi.fn(),
        onClick: vi.fn(),
      },
    }),
    stageBindingFor: () => null,
  }).forNode(nodeId)
}

describe('FlowCanvasNode per-node uiState subscription', () => {
  it('resolves uiState from the injected context and skips sibling re-renders', async () => {
    fusionRenderCounts.clear()
    const entryA = ref<MarkedUiState>(markedUiState('a1'))
    const entryB = ref<MarkedUiState>(markedUiState('b1'))
    // Mirrors the shared cache semantics: one computed Record over all
    // nodes, with stable per-node object identity for unchanged nodes.
    const cache = computed<Record<string, MarkedUiState>>(() => ({
      'fusion-1': entryA.value,
      'fusion-2': entryB.value,
    }))
    const fallback = markedUiState('fallback')

    const Host = defineComponent({
      setup() {
        provideCanvasNodeUiStateContext({
          getFlowNodeUiState: (nodeId: string) => cache.value[nodeId] ?? fallback,
        })
        return () =>
          h('div', [
            h(FlowCanvasNode, {
              nodeId: 'fusion-1',
              data: createFusionData(createFusionSnapshot('Fusion One')),
              imagePresentation: createImagePresentation('fusion-1'),
            }),
            h(FlowCanvasNode, {
              nodeId: 'fusion-2',
              data: createFusionData(createFusionSnapshot('Fusion Two')),
              imagePresentation: createImagePresentation('fusion-2'),
            }),
          ])
      },
    })
    const view = render(Host, {
      global: {
        stubs: {
          FusionNode: FusionProbe,
          FlowCanvasNodeHandleRail: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    expect(view.getByTestId('fusion-probe-Fusion One')).toHaveTextContent('"a1"')
    expect(view.getByTestId('fusion-probe-Fusion Two')).toHaveTextContent('"b1"')
    expect(fusionRenderCounts.get('Fusion One')).toBe(1)
    expect(fusionRenderCounts.get('Fusion Two')).toBe(1)

    entryA.value = markedUiState('a2')
    await nextTick()

    expect(view.getByTestId('fusion-probe-Fusion One')).toHaveTextContent('"a2"')
    expect(view.getByTestId('fusion-probe-Fusion Two')).toHaveTextContent('"b1"')
    expect(fusionRenderCounts.get('Fusion One')).toBe(2)
    expect(fusionRenderCounts.get('Fusion Two')).toBe(1)
  })

  it('falls back to an explicit uiState prop when provided', async () => {
    const Host = defineComponent({
      setup() {
        provideCanvasNodeUiStateContext({
          getFlowNodeUiState: () => markedUiState('from-context'),
        })
        return () =>
          h(FlowCanvasNode, {
            nodeId: 'fusion-1',
            data: createFusionData(createFusionSnapshot('Fusion One')),
            uiState: markedUiState('from-prop'),
            imagePresentation: createImagePresentation('fusion-1'),
          })
      },
    })
    const view = render(Host, {
      global: {
        stubs: {
          FusionNode: FusionProbe,
          FlowCanvasNodeHandleRail: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    expect(view.getByTestId('fusion-probe-Fusion One')).toHaveTextContent('"from-prop"')
  })
})
