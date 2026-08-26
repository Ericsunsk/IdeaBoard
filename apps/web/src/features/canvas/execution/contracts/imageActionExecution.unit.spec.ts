import { describe, expect, it } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  IMAGE_ACTION_EXECUTION_REGISTRY,
  IMAGE_ACTION_EXECUTION_REGISTRY_LIST,
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_DIRECT_TASK_ACTIONS,
  IMAGE_EXECUTION_MODE,
  IMAGE_HD_UPSCALE_ACTION,
  isImageDirectTaskAction,
  isImageStrengthAction,
  isImageTaskAction,
} from './imageActionExecution'

describe('imageActionExecution', () => {
  it('defines executable image actions without UI metadata', () => {
    const actions = IMAGE_ACTION_EXECUTION_REGISTRY_LIST.map((item) => item.action)

    expect(actions).toEqual([
      IMAGE_NODE_ACTION.cutout,
      IMAGE_NODE_ACTION.variations,
      IMAGE_NODE_ACTION.render,
      IMAGE_NODE_ACTION.hdUpscale,
      IMAGE_NODE_ACTION.outpaint,
    ])
    for (const item of IMAGE_ACTION_EXECUTION_REGISTRY_LIST) {
      expect(item).not.toHaveProperty('label')
      expect(item).not.toHaveProperty('title')
      expect(item).not.toHaveProperty('icon')
    }
  })

  it('derives task and strength action contracts', () => {
    expect(IMAGE_DIRECT_TASK_ACTIONS).toEqual([
      IMAGE_NODE_ACTION.cutout,
      IMAGE_NODE_ACTION.variations,
      IMAGE_NODE_ACTION.render,
    ])
    expect(isImageDirectTaskAction(IMAGE_NODE_ACTION.cutout)).toBe(true)
    expect(isImageDirectTaskAction(IMAGE_HD_UPSCALE_ACTION)).toBe(false)
    expect(isImageTaskAction(IMAGE_HD_UPSCALE_ACTION)).toBe(true)
    expect(isImageTaskAction(IMAGE_CANVAS_EXTEND_ACTION)).toBe(false)
    expect(isImageStrengthAction(IMAGE_NODE_ACTION.variations)).toBe(true)
    expect(isImageStrengthAction(IMAGE_HD_UPSCALE_ACTION)).toBe(true)
    expect(DEFAULT_IMAGE_ACTION_STRENGTHS).toEqual({
      [IMAGE_NODE_ACTION.variations]: DEFAULT_IMAGE_ACTION_STRENGTH,
      [IMAGE_HD_UPSCALE_ACTION]: 2,
    })
  })

  it('maps execution modes and features by action', () => {
    expect(IMAGE_ACTION_EXECUTION_REGISTRY[IMAGE_NODE_ACTION.cutout]).toMatchObject({
      mode: IMAGE_EXECUTION_MODE.directTask,
      feature: IMAGE_NODE_ACTION.cutout,
    })
    expect(IMAGE_ACTION_EXECUTION_REGISTRY[IMAGE_HD_UPSCALE_ACTION]).toMatchObject({
      mode: IMAGE_EXECUTION_MODE.hdUpscaleTask,
      feature: IMAGE_HD_UPSCALE_ACTION,
    })
    expect(IMAGE_ACTION_EXECUTION_REGISTRY[IMAGE_CANVAS_EXTEND_ACTION]).toMatchObject({
      mode: IMAGE_EXECUTION_MODE.canvasExtend,
      feature: IMAGE_CANVAS_EXTEND_ACTION,
    })
  })
})
