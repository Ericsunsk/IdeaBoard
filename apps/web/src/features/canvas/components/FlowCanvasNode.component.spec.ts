import { describe, expect, it, vi } from 'vitest'
import FlowCanvasNode from './FlowCanvasNode.vue'
import type { PromptNodeItem } from '../../canvas-board/domain'
import type { ImageActionBinding } from '../image-tools'
import type { FlowNodeData } from '../types/flowTypes'
import { render } from '../../../test/render'

function createPromptSnapshot(overrides: Partial<PromptNodeItem> = {}): PromptNodeItem {
  return {
    id: 1,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    title: 'Prompt',
    text: 'A benchmark prompt',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
    ...overrides,
  }
}

function createPromptData(snapshot: PromptNodeItem): FlowNodeData {
  return {
    kind: 'prompt',
    nodeType: 'prompt',
    title: snapshot.title ?? 'Prompt',
    snapshotNode: snapshot,
  }
}

function createImageActionBinding(): ImageActionBinding {
  return {
    actionMenuVisible: false,
    promptComposerVisible: false,
    promptPlaceholder: '',
    directGenerating: false,
    showStrengthControl: false,
    activeStrengthAction: null,
    getStrengthValue: () => 50,
    setStrengthRangeRef: vi.fn(),
    setStrengthTriggerRef: vi.fn(),
    onStrengthRangePointerDown: vi.fn(),
    onStrengthInput: vi.fn(),
    onStrengthChange: vi.fn(),
    onStrengthPointerUp: vi.fn(),
    onStrengthClick: vi.fn(),
  }
}

function renderFlowCanvasNode(
  input: {
    selected?: boolean
    dragging?: boolean
    lightweightNodeRendering?: boolean
  } = {},
) {
  const snapshot = createPromptSnapshot()
  return render(FlowCanvasNode, {
    props: {
      nodeId: 'prompt-1',
      data: createPromptData(snapshot),
      selected: input.selected ?? false,
      dragging: input.dragging ?? false,
      copyMode: false,
      imageActionBinding: createImageActionBinding(),
      lightweightNodeRendering: input.lightweightNodeRendering ?? false,
    },
    global: {
      stubs: {
        FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
      },
    },
  })
}

describe('FlowCanvasNode lightweight rendering', () => {
  it('renders a lightweight body for non-interactive nodes when enabled', () => {
    const view = renderFlowCanvasNode({ lightweightNodeRendering: true })

    expect(view.container.querySelector('[data-canvas-lightweight-node="true"]')).not.toBeNull()
    expect(view.container.querySelector('.node-stage-frame--prompt')).toBeNull()
  })

  it('keeps the full node body for selected nodes at rest', () => {
    const view = renderFlowCanvasNode({
      selected: true,
      lightweightNodeRendering: true,
    })

    expect(view.container.querySelector('[data-canvas-lightweight-node="true"]')).toBeNull()
    expect(view.container.querySelector('.node-stage-frame--prompt')).not.toBeNull()
  })

  it('uses the lightweight body while a selected node is dragging', () => {
    const view = renderFlowCanvasNode({
      selected: true,
      dragging: true,
      lightweightNodeRendering: true,
    })

    expect(view.container.querySelector('[data-canvas-lightweight-node="true"]')).not.toBeNull()
  })
})
