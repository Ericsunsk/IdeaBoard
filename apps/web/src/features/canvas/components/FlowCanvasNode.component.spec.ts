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
  }
}

function renderFlowCanvasNode(
  input: {
    selected?: boolean
    dragging?: boolean
    generationAvailable?: boolean
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
      generationAvailable: input.generationAvailable ?? true,
      imageActionBinding: createImageActionBinding(),
    },
    global: {
      stubs: {
        FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
      },
    },
  })
}

describe('FlowCanvasNode prompt rendering', () => {
  it('renders the full prompt body for non-interactive nodes', () => {
    const view = renderFlowCanvasNode()

    expect(view.container.querySelector('.node-stage-frame--prompt')).not.toBeNull()
    expect(view.getByLabelText('发送')).not.toBeDisabled()
  })

  it('keeps the full prompt body for selected nodes at rest', () => {
    const view = renderFlowCanvasNode({
      selected: true,
    })

    expect(view.container.querySelector('.node-stage-frame--prompt')).not.toBeNull()
  })

  it('keeps the full prompt body while a selected node is dragging', () => {
    const view = renderFlowCanvasNode({
      selected: true,
      dragging: true,
    })

    expect(view.container.querySelector('.node-stage-frame--prompt')).not.toBeNull()
  })

  it('disables prompt submission while generation is unavailable', () => {
    const view = renderFlowCanvasNode({
      generationAvailable: false,
    })

    expect(view.getByLabelText('发送')).toBeDisabled()
  })
})
