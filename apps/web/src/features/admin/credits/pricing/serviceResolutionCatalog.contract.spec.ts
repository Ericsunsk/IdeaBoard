import { describe, expect, it } from 'vitest'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  parseOutputResolution,
} from '../../../../core/contracts/generationContract'
import { SERVICE, SERVICE_SLUGS } from '../../../../core/contracts/imageServices'

// #856 service + resolution catalog contract: Canvas, Admin Prompt,
// Pricing, and Model Pool read these two shared sources instead of local
// copies. Downstream tickets (#859/#860/#863) must consume exactly:
//   services   -> apps/web/src/core/contracts/imageServices.ts (SERVICE)
//   resolutions -> apps/web/src/core/contracts/generationContract.ts
//                 (OUTPUT_RESOLUTION_OPTIONS, parseOutputResolution)
describe('service resolution catalog contract', () => {
  it('keeps the service catalog on the eight workflow slugs', () => {
    expect([...SERVICE_SLUGS]).toEqual([
      SERVICE.generation,
      SERVICE.inpaint,
      SERVICE.cutout,
      SERVICE.fusion,
      SERVICE.render,
      SERVICE.variations,
      SERVICE.upscale,
      SERVICE.outpaint,
    ])
    expect(SERVICE_SLUGS).not.toContain('vision')
    expect(SERVICE_SLUGS).not.toContain('default')
  })

  it('derives billing SKUs as ai-image:<service> with no model identity', () => {
    for (const slug of SERVICE_SLUGS) {
      expect(`ai-image:${slug}`).toMatch(/^ai-image:[a-z]+$/)
    }
  })

  it('keeps the resolution catalog on exactly 1k, 2k, and 4k', () => {
    expect([...OUTPUT_RESOLUTION_OPTIONS]).toEqual(['1k', '2k', '4k'])
    expect(OUTPUT_RESOLUTION_OPTIONS).not.toContain('default')
    for (const resolution of OUTPUT_RESOLUTION_OPTIONS) {
      expect(parseOutputResolution(resolution)).toBe(resolution)
    }
    for (const value of ['default', 'DEFAULT', '', '  ', '8k', '1.5k', '1 k', null, undefined]) {
      expect(parseOutputResolution(value)).toBeNull()
    }
    expect(parseOutputResolution('1K')).toBe('1k')
    expect(parseOutputResolution(' 2k ')).toBe('2k')
  })
})
