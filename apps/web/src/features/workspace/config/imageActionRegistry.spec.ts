import { describe, expect, it } from 'vitest'
import {
  IMAGE_ACTION_MENU_ITEMS,
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  IMAGE_ACTION_REGISTRY_LIST,
  IMAGE_NODE_ACTION,
} from './imageActionRegistry'
import {
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  isImageStrengthAction,
} from '../execution'

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
      IMAGE_NODE_ACTION.localImageEditor,
    ])
  })

  it('derives menu items from the registry', () => {
    expect(IMAGE_ACTION_MENU_ITEMS).toEqual(
      IMAGE_ACTION_REGISTRY_LIST.map((item) => ({
        action: item.action,
        label: item.label,
        title: item.title,
        icon: item.icon,
        ...(isImageStrengthAction(item.action) ? { strengthAction: item.action } : {}),
      })),
    )
  })

  it('does not expose submit authority fields that runtime does not consume', () => {
    const unusedAuthorityKeys = [
      'execution',
      'strength',
      'prompt',
      'input',
      'result',
      'readiness',
    ] as const

    for (const item of IMAGE_ACTION_REGISTRY_LIST) {
      for (const key of unusedAuthorityKeys) {
        expect(item).not.toHaveProperty(key)
      }
    }
  })

  it('keeps task and local tool modes explicit', () => {
    expect(IMAGE_ACTION_REGISTRY[IMAGE_HD_UPSCALE_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.hdUpscaleTask,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_CANVAS_EXTEND_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.canvasExtend,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.imageInpaint].mode).toBe(
      IMAGE_ACTION_MODE.localInpaint,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.localImageEditor].mode).toBe(
      IMAGE_ACTION_MODE.localImageEditor,
    )
  })
})
