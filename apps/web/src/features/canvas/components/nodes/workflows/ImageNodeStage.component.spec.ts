import { computed, toValue } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import type { ImageToolStageBinding } from '../../../image-tools'
import ImageNodeStage from './ImageNodeStage.vue'
import { render } from '../../../../../test/render'

vi.mock('../../../vue-flow-adapter/useDisplayImagePreview', () => ({
  useDisplayImagePreview: (sourceUrl: Parameters<typeof toValue>[0]) => ({
    displayUrl: computed(() => toValue(sourceUrl) ?? ''),
  }),
}))

function renderImageStage(toolActive: boolean) {
  return render(ImageNodeStage, {
    props: {
      nodeId: 'image-1',
      stageWidth: 280,
      stageHeight: 280,
      displayImage: { url: '' },
      imageToolStageBinding: { active: toolActive } as ImageToolStageBinding,
      active: false,
      dragging: false,
    },
    global: {
      stubs: {
        ImageToolStagePresentation: { template: '<div><slot /></div>' },
        NodeStageFrame: {
          props: ['transparent'],
          template:
            '<div data-testid="node-stage-frame" :data-transparent="String(transparent)"><slot /></div>',
        },
      },
    },
  })
}

describe('ImageNodeStage disabled state', () => {
  it('disables file selection while an image action is generating', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: null,
        directGenerating: true,
        active: false,
        dragging: false,
      },
      global: {
        stubs: {
          NodeStageFrame: { template: '<div><slot /></div>' },
        },
      },
    })

    expect(view.getByLabelText('图片正在生成，暂时无法上传')).toBeTruthy()
    expect(view.getByLabelText('图片正在生成，暂时无法上传').tagName).not.toBe('BUTTON')
  })
})

describe('ImageNodeStage Library import recovery', () => {
  it('keeps an optimistic preview visible while the Library import materializes', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/preview.png' },
        libraryImport: { state: 'pending', libraryAssetId: 7 },
        active: false,
        dragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /图片节点预览/ })).toHaveAttribute(
      'src',
      'https://provider.example/preview.png',
    )
  })

  it('offers Retry only for a failed persisted Library import', async () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: null,
        libraryImport: {
          state: 'failed',
          libraryAssetId: 7,
          failureMessage: '素材导入失败',
        },
        active: false,
        dragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    await fireEvent.click(view.getByLabelText('重试导入图片'))
    expect(view.emitted('retry-image-import')).toEqual([[]])
  })

  it('offers the same compact Retry action for a failed local upload', async () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'blob:local-image' },
        localUpload: { previewUrl: 'blob:local-image', failureMessage: '上传失败' },
        active: false,
        dragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /图片节点预览/ })).toHaveAttribute(
      'src',
      'blob:local-image',
    )
    await fireEvent.click(view.getByLabelText('重试导入图片'))
    expect(view.emitted('retry-image-import')).toEqual([[]])
  })
})

describe('ImageNodeStage image tool presentation', () => {
  it('keeps the stage transparent and hides stage actions only while the tool binding is active', () => {
    const activeTool = renderImageStage(true)
    const noTool = renderImageStage(false)
    expect(activeTool.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'true',
    )
    expect(activeTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
    expect(noTool.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'false',
    )
    expect(noTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
  })
})
