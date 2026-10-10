import { describe, expect, it } from 'vitest'
import { SERVICE } from '../../../core/contracts/imageServices'
import { getServiceImageActionText } from './serviceImageActionText'

describe('service image action text', () => {
  it('exposes canvas image action text without feature modules rebuilding it', () => {
    expect(getServiceImageActionText(SERVICE.cutout)).toEqual({
      label: 'Cutout',
      title: 'Cutout',
    })
    expect(getServiceImageActionText(SERVICE.inpaint)).toEqual({
      label: 'Inpaint',
      title: 'Inpaint',
    })
  })

  it('accepts a caller-supplied label resolver instead of reading i18n itself', () => {
    const seenKeys: string[] = []
    const text = getServiceImageActionText(SERVICE.cutout, (key) => {
      seenKeys.push(key)
      return `resolved:${key}`
    })
    expect(seenKeys).toEqual(['canvas.features.cutout'])
    expect(text).toEqual({
      label: 'resolved:canvas.features.cutout',
      title: 'resolved:canvas.features.cutout',
    })
  })
})
