import { fireEvent } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'
import FlowCanvasNode from './FlowCanvasNode.vue'
import type { FusionNodeItem, ImageNodeItem, PromptNodeItem } from '../../canvas-board/domain'
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

function createImagePresentation(overrides: Partial<ImageActionBinding> = {}) {
  const actionBinding = { ...createImageActionBinding(), ...overrides }
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

function createFusionData(snapshot: FusionNodeItem): FlowNodeData {
  return {
    kind: 'fusion',
    nodeType: 'fusion',
    title: snapshot.title ?? 'Fusion',
    snapshotNode: snapshot,
  }
}

function createFusionSnapshot(overrides: Partial<FusionNodeItem> = {}): FusionNodeItem {
  return {
    id: 1,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 280,
    title: 'Fusion',
    prompt: '',
    outputCount: 1,
    aspectRatio: 'auto',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
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
    expect(
      view.container
        .querySelector<HTMLElement>('.flow-canvas-node')
        ?.style.getPropertyValue('--canvas-node-ui-scale'),
    ).toBe('1')
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

    expect(view.emitted('node-intent')).toContainEqual([
      { type: 'text-edited', nodeId: 'prompt-1', text: '新的提示词' },
    ])
  })

  it('emits a semantic presentation intent when prompt generation is requested', async () => {
    const view = renderFlowCanvasNode()

    await fireEvent.click(view.getByLabelText('发送'))

    expect(view.emitted('node-intent')).toContainEqual([
      {
        type: 'prompt-generation-requested',
        nodeId: 'prompt-1',
        prompt: 'A benchmark prompt',
        outputCount: 1,
        aspectRatio: 'auto',
      },
    ])
  })
})

describe('FlowCanvasNode image presentation', () => {
  it('anchors the context tools and prompt composer in one screen-sized container', () => {
    const view = render(FlowCanvasNode, {
      props: {
        nodeId: 'image-1',
        data: createImageData(createImageSnapshot()),
        imagePresentation: createImagePresentation({
          actionMenuVisible: true,
          promptPlaceholder: '输入提示词改造这张图',
          promptComposerVisible: true,
        }),
      },
      global: {
        stubs: {
          FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    const floatingControls = view.container.querySelector('.node-floating-controls')

    expect(floatingControls?.querySelector('.image-action-menu')).not.toBeNull()
    expect(floatingControls?.querySelector('.image-node-prompt-composer')).not.toBeNull()
    expect(view.getByPlaceholderText('输入提示词改造这张图')).toHaveClass('self-center')
    expect(view.getByLabelText('打开图片预览').closest('.node-floating-controls')).toBeNull()
  })

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

    expect(view.emitted('node-intent')).toEqual(
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

    expect(view.emitted('node-intent')).toContainEqual([
      { type: 'file-open-requested', nodeId: 'image-1' },
    ])
  })
})

describe('FlowCanvasNode fusion presentation', () => {
  it('emits the shared presentation intent for Fusion parameter changes', async () => {
    const view = render(FlowCanvasNode, {
      props: {
        nodeId: 'fusion-1',
        data: createFusionData(createFusionSnapshot()),
        imagePresentation: createImagePresentation(),
      },
      global: {
        stubs: {
          FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    const title = view.getByTitle('单击重命名')
    await fireEvent.click(title)
    title.textContent = '新的融合节点'
    await fireEvent.input(title)
    await fireEvent.blur(title)

    expect(view.emitted('node-intent')).toContainEqual([
      {
        type: 'parameter-changed',
        nodeId: 'fusion-1',
        patch: { key: 'title', value: '新的融合节点' },
      },
    ])
  })

  it('emits shared intents for Fusion input, context, and prompt generation', async () => {
    const view = render(FlowCanvasNode, {
      props: {
        nodeId: 'fusion-1',
        data: createFusionData(
          createFusionSnapshot({
            inputImages: [
              {
                id: 'image-1',
                url: 'https://example.com/one.png',
                ratio: 1,
                sourceType: 'external',
                sourceId: 'media-1',
              },
              {
                id: 'image-2',
                url: 'https://example.com/two.png',
                ratio: 1,
                sourceType: 'external',
                sourceId: 'media-2',
              },
            ],
          }),
        ),
        fusionPresentation: { promptComposerVisible: true },
        imagePresentation: createImagePresentation(),
      },
      global: {
        stubs: {
          FlowNodeHandles: { template: '<div data-testid="flow-node-handles" />' },
        },
      },
    })

    const stage = view.container.querySelector('.fusion-node-stage')!
    const floatingControls = view.container.querySelector('.node-floating-controls')

    expect(floatingControls?.querySelector('.image-node-prompt-composer')).not.toBeNull()
    expect(view.getByPlaceholderText('输入融合提示词')).toHaveClass('self-center')

    await fireEvent.drop(stage)
    await fireEvent.contextMenu(stage)
    await fireEvent.click(view.getAllByLabelText('移除图片输入')[0]!)
    await fireEvent.update(view.getByPlaceholderText('输入融合提示词'), '融合两张图片')
    await fireEvent.click(view.getByLabelText('发送'))

    expect(view.emitted('node-intent')).toEqual(
      expect.arrayContaining([
        [expect.objectContaining({ type: 'media-input-dropped' })],
        [expect.objectContaining({ type: 'fusion-context-menu-requested' })],
        [
          {
            type: 'media-input-remove-requested',
            nodeId: 'fusion-1',
            imageId: 'image-1',
          },
        ],
        [
          {
            type: 'fusion-generation-requested',
            nodeId: 'fusion-1',
            prompt: '融合两张图片',
            outputCount: 1,
            aspectRatio: 'auto',
          },
        ],
      ]),
    )
  })
})
