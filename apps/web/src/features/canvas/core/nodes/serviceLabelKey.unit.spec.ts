import { describe, expect, it } from 'vitest'
import { SERVICE, SERVICE_SLUGS } from '../../../../core/contracts/imageServices'
import { serviceLabelKey } from './serviceLabelKey'

describe('service label keys', () => {
  it('mirrors every service slug under canvas.features', () => {
    for (const slug of SERVICE_SLUGS) {
      expect(serviceLabelKey(slug)).toBe(`canvas.features.${slug}`)
    }
  })

  it('derives the cutout locale key from the slug', () => {
    expect(serviceLabelKey(SERVICE.cutout)).toBe('canvas.features.cutout')
  })
})
