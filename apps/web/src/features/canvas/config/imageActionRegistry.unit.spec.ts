import { describe, expect, it } from 'vitest'
import {
  getImageActionMenuItems,
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  IMAGE_ACTION_REGISTRY_LIST,
  IMAGE_NODE_ACTION,
} from './imageActionRegistry'
import {
  IMAGE_OUTPAINT_ACTION,
  IMAGE_UPSCALE_ACTION,
  isImageStrengthAction,
} from '../execution'

describe('imageActionRegistry', () => {
  it('defines each image action exactly once', () => {
    const actions = IMAGE_ACTION_REGISTRY_LIST.map((item) => item.action)

    expect(new Set(actions).size).toBe(actions.length)
    expect(actions).toEqual([
      IMAGE_NODE_ACTION.cutout,
      IMAGE_NODE_ACTION.variations,
      IMAGE_NODE_ACTION.render,
      IMAGE_NODE_ACTION.upscale,
      IMAGE_NODE_ACTION.outpaint,
      IMAGE_NODE_ACTION.inpaint,
      IMAGE_NODE_ACTION.retouch,
    ])
  })

  it('derives menu items from the registry with copy resolved read-time', () => {
    const items = getImageActionMenuItems()
    expect(items.map((item) => item.label)).toEqual([
      'Cutout',
      'Variations',
      'Render',
      'Upscale',
      'Outpaint',
      'Inpaint',
      'Retouch',
    ])
    expect(
      items
        .filter((item) => isImageStrengthAction(item.action))
        .map((item) => item.strengthAction),
    ).toEqual([IMAGE_NODE_ACTION.variations, IMAGE_NODE_ACTION.upscale])
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
    expect(IMAGE_ACTION_REGISTRY[IMAGE_UPSCALE_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.upscaleTask,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_OUTPAINT_ACTION].mode).toBe(
      IMAGE_ACTION_MODE.outpaint,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.inpaint].mode).toBe(
      IMAGE_ACTION_MODE.inpaint,
    )
    expect(IMAGE_ACTION_REGISTRY[IMAGE_NODE_ACTION.retouch].mode).toBe(
      IMAGE_ACTION_MODE.retouch,
    )
  })
})
