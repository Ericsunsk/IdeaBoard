import { describe, expect, it } from 'vitest'
import { serviceOfSku } from './skuFeature'

describe('serviceOfSku', () => {
  it('extracts the service from image SKUs', () => {
    expect(serviceOfSku('ai-image:inpaint')).toBe('inpaint')
  })

  it('returns null for non-image SKUs instead of a shared empty key', () => {
    expect(serviceOfSku('ai-search:vision')).toBeNull()
    expect(serviceOfSku('')).toBeNull()
    expect(serviceOfSku('ai-image:')).toBeNull()
  })
})
