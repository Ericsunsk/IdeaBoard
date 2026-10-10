import { describe, expect, it } from 'vitest'
import { SERVICE } from '../../../../core/contracts/imageServices'
import { getServicePromptVariables } from './promptVariables'

describe('service prompt variables', () => {
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
})
