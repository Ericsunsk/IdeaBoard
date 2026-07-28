import { fireEvent } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'
import FlowCanvasNode from './FlowCanvasNode.vue'
import type { ImageNodeItem, PromptNodeItem } from '../../canvas-board/domain'
import type { ImageActionBinding } from '../image-tools'
import { createImageNodePresentationResolver } from '../node-presentation'
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

function createImagePresentation() {
  const actionBinding = createImageActionBinding()
  return createImageNodePresentationResolver({
    actionBindingFor: () => actionBinding,
    stageBindingFor: () => null,
  }).forNode('image-1')
}

function createImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function createImageData(snapshot: ImageNodeItem): FlowNodeData {
  return {
    kind: 'image',
    nodeType: 'image',
    title: snapshot.title ?? 'Image',
    snapshotNode: snapshot,
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
      imagePresentation: createImagePresentation(),
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

  it('emits a semantic presentation intent when prompt text is edited', async () => {
    const view = renderFlowCanvasNode()

    await fireEvent.update(view.getByPlaceholderText('type your idea...'), '新的提示词')

    expect(view.emitted('prompt-intent')).toContainEqual([
      { type: 'text-edited', nodeId: 'prompt-1', text: '新的提示词' },
    ])
  })

  it('emits a semantic presentation intent when prompt generation is requested', async () => {
    const view = renderFlowCanvasNode()

    await fireEvent.click(view.getByLabelText('发送'))

    expect(view.emitted('prompt-intent')).toContainEqual([
      {
        type: 'generation-requested',
        nodeId: 'prompt-1',
        prompt: 'A benchmark prompt',
        outputCount: 1,
        aspectRatio: 'auto',
      },
    ])
  })
})

describe('FlowCanvasNode image presentation', () => {
  it('emits semantic intents for upload, preview, append, and reset', async () => {
    const view = render(FlowCanvasNode, {
      props: {
        nodeId: 'image-1',
        data: createImageData(createImageSnapshot()),
        imagePresentation: createImagePresentation(),
      },
      global: {
        stubs: {
          FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    await fireEvent.click(view.getByLabelText('打开图片预览'))
    await fireEvent.dblClick(view.getByAltText('图片节点预览'))
    await fireEvent.click(view.getByLabelText('清空图片输入'))

    expect(view.emitted('image-intent')).toEqual(
      expect.arrayContaining([
        [{ type: 'preview-requested', nodeId: 'image-1' }],
        [{ type: 'append-to-canvas-requested', nodeId: 'image-1' }],
        [{ type: 'reset-requested', nodeId: 'image-1' }],
      ]),
    )
  })

  it('emits a semantic intent when an empty image node requests an upload', async () => {
    const view = render(FlowCanvasNode, {
      props: {
        nodeId: 'image-1',
        data: createImageData(createImageSnapshot({ sourceImageUrl: '' })),
        imagePresentation: createImagePresentation(),
      },
      global: {
        stubs: {
          FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    await fireEvent.click(view.getByLabelText('上传图片作为输入'))

    expect(view.emitted('image-intent')).toContainEqual([
      { type: 'file-open-requested', nodeId: 'image-1' },
    ])
  })
})
