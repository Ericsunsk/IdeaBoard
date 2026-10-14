import { describe, expect, it } from 'vitest'
import {
  BorderInner,
  ChatLines,
  EditPencil,
  Expand,
  Keyframes,
  Palette,
  Spark,
  ZoomIn,
} from '@iconoir/vue'
import { SERVICE, SERVICE_SLUGS } from '../contracts/imageServices'
import { getServiceIcon } from './serviceIcons'

describe('getServiceIcon', () => {
  it('uses the user-facing icon for each service across surfaces', () => {
    expect(getServiceIcon(SERVICE.render)).toBe(Palette)
    expect(getServiceIcon(SERVICE.variations)).toBe(Spark)
    expect(getServiceIcon(SERVICE.inpaint)).toBe(EditPencil)
    expect(getServiceIcon(SERVICE.cutout)).toBe(BorderInner)
    expect(getServiceIcon(SERVICE.fusion)).toBe(Keyframes)
    expect(getServiceIcon(SERVICE.upscale)).toBe(ZoomIn)
    expect(getServiceIcon(SERVICE.outpaint)).toBe(Expand)
    expect(getServiceIcon(SERVICE.generation)).toBe(ChatLines)
  })

  it('covers every service slug and ignores unknown values', () => {
    expect(SERVICE_SLUGS.every((slug) => getServiceIcon(slug))).toBe(true)
    expect(getServiceIcon('RENDER')).toBe(Palette)
    expect(getServiceIcon('unknown')).toBeUndefined()
    expect(getServiceIcon('constructor')).toBeUndefined()
    expect(getServiceIcon('__proto__')).toBeUndefined()
    expect(getServiceIcon('')).toBeUndefined()
  })
})
