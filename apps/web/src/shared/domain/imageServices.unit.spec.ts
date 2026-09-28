import { describe, expect, it } from 'vitest'
import { SERVICE, SERVICE_SLUGS } from './imageServices'

describe('workflow feature registry', () => {
  it('keeps the production allowlist on the clean-break English identities', () => {
    expect(SERVICE_SLUGS).toEqual(expect.arrayContaining([SERVICE.generation, SERVICE.inpaint]))
    expect(SERVICE_SLUGS).not.toContain('local-inpaint')
    expect(SERVICE_SLUGS).not.toContain('image-extend')
    expect(SERVICE_SLUGS).not.toContain('pattern-gen')
  })
})
