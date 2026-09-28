import { describe, expect, it } from 'vitest'
import { SERVICE } from './imageServices'
import { getServicePromptVariables, getServiceImageActionText } from './serviceRegistry'

describe('serviceRegistry projections', () => {
  it('publishes prompt variables from the workflow code registry', () => {
    expect(getServicePromptVariables(SERVICE.variations)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{variation_strength}}', tone: 'control' },
    ])
    expect(getServicePromptVariables(SERVICE.fusion)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{fusion_input_weights}}', tone: 'control' },
    ])
    expect(getServicePromptVariables(SERVICE.cutout)).toEqual([])
  })

  it('hides user_prompt when fill providers receive it separately', () => {
    expect(getServicePromptVariables(SERVICE.inpaint)).toEqual([])
    expect(getServicePromptVariables(SERVICE.outpaint)).toEqual([])
  })

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
