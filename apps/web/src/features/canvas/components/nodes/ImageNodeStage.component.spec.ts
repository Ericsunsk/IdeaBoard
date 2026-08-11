import { describe, expect, it } from 'vitest'
import type { ImageToolStageBinding } from '../../image-tools'
import ImageNodeStage from './ImageNodeStage.vue'
import { render } from '../../../../test/render'

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

describe('ImageNodeStage image tool presentation', () => {
  it('keeps the stage transparent and hides stage actions only while the tool binding is active', () => {
    const activeTool = renderImageStage(true)
    const noTool = renderImageStage(false)
    const activeFrame = activeTool.container.querySelector('.node-stage-frame')
    const inactiveFrame = noTool.container.querySelector('.node-stage-frame')

    expect(activeFrame).toHaveClass('image-node-stage', 'is-image-editor-active', 'is-transparent')
    expect(activeTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
    expect(inactiveFrame).toHaveClass('image-node-stage')
    expect(inactiveFrame).not.toHaveClass('is-image-editor-active', 'is-transparent')
    expect(noTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
  })
})
