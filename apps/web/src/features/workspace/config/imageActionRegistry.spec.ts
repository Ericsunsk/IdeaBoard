import { describe, expect, it } from 'vitest'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  IMAGE_ACTION_MENU_ITEMS,
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  IMAGE_ACTION_REGISTRY_LIST,
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  IMAGE_NODE_ACTION,
  isImageDirectTaskAction,
  isImageStrengthAction,
} from './imageActionRegistry'

describe('imageActionRegistry', () => {
  it('defines each image action exactly once', () => {
    const actions = IMAGE_ACTION_REGISTRY_LIST.map((item) => item.action)

    expect(new Set(actions).size).toBe(actions.length)
    expect(actions).toEqual([
      IMAGE_NODE_ACTION.removeBg,
      IMAGE_NODE_ACTION.styleVariation,
      IMAGE_NODE_ACTION.sketchToImage,
      IMAGE_NODE_ACTION.hdUpscale,
      IMAGE_NODE_ACTION.imageExtend,
      IMAGE_NODE_ACTION.imageInpaint,
    ])
  })

  it('derives menu items from the registry', () => {
    expect(IMAGE_ACTION_MENU_ITEMS).toEqual(
      IMAGE_ACTION_REGISTRY_LIST.map((item) => ({
        action: item.action,
        label: item.label,
        title: item.title,
        icon: item.icon,
        ...('strength' in item ? { strengthAction: item.action } : {}),
      })),
    )
  })

  it('derives strength actions and defaults from registry metadata', () => {
    expect(isImageStrengthAction(IMAGE_NODE_ACTION.styleVariation)).toBe(true)
    expect(isImageStrengthAction(IMAGE_NODE_ACTION.hdUpscale)).toBe(true)
    expect(isImageStrengthAction(IMAGE_NODE_ACTION.removeBg)).toBe(false)
    expect(DEFAULT_IMAGE_ACTION_STRENGTHS).toEqual({
      [IMAGE_NODE_ACTION.styleVariation]: DEFAULT_IMAGE_ACTION_STRENGTH,
      [IMAGE_NODE_ACTION.hdUpscale]: DEFAULT_IMAGE_ACTION_STRENGTH,
    })
  })

  it('does not expose submit authority fields that runtime does not consume', () => {
    const unusedAuthorityKeys = ['prompt', 'input', 'result', 'readiness'] as const

    for (const item of IMAGE_ACTION_REGISTRY_LIST) {
      for (const key of unusedAuthorityKeys) {
        expect(item).not.toHaveProperty(key)
      }
    }
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.imageInpaint]).not.toHaveProperty('execution')
  })

  it('keeps task and local tool modes explicit', () => {
    expect(isImageDirectTaskAction(IMAGE_NODE_ACTION.removeBg)).toBe(true)
    expect(isImageDirectTaskAction(IMAGE_NODE_ACTION.styleVariation)).toBe(true)
    expect(isImageDirectTaskAction(IMAGE_NODE_ACTION.sketchToImage)).toBe(true)
    expect(IMAGE_ACTION_REGISTRY[IMAGE_HD_UPSCALE_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.hdUpscaleTask,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_CANVAS_EXTEND_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.canvasExtend,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.imageInpaint].mode).toBe(
      IMAGE_ACTION_MODE.localInpaint,
    )
  })
})
