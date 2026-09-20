import { describe, expect, it } from 'vitest'
import { featureOfSku } from './skuFeature'

describe('featureOfSku', () => {
  it('extracts the workflow feature from image SKUs', () => {
    expect(featureOfSku('ai-image:inpaint')).toBe('inpaint')
  })

  it('returns null for non-image SKUs instead of a shared empty key', () => {
    expect(featureOfSku('ai-search:vision')).toBeNull()
    expect(featureOfSku('')).toBeNull()
    expect(featureOfSku('ai-image:')).toBeNull()
  })
})
