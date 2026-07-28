import { describe, expect, it } from 'vitest'
import ImageNodeStage from './ImageNodeStage.vue'
import { render } from '../../../../test/render'

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
