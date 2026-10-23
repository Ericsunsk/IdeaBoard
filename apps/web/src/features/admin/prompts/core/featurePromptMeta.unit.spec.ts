import { describe, expect, it } from 'vitest'
import { SERVICE, SERVICE_SLUGS } from '@/core/contracts/imageServices'
import { featureMap, getFeaturePromptStatus, groupedFeatureGroups } from './featurePromptMeta'

describe('featurePromptMeta', () => {
  it('keeps prompt configuration labels and grouping within the Admin boundary', () => {
    expect(
      groupedFeatureGroups.map((group) => ({
        key: group.key,
        title: group.title,
        order: group.order,
        slugs: group.items.map((item) => item.slug),
      })),
    ).toEqual([
      {
        key: 'image',
        title: 'Image',
        order: 10,
        slugs: [
          SERVICE.render,
          SERVICE.variations,
          SERVICE.inpaint,
          SERVICE.outpaint,
          SERVICE.upscale,
          SERVICE.cutout,
        ],
      },
      {
        key: 'prompt',
        title: 'Prompt',
        order: 20,
        slugs: [SERVICE.generation],
      },
      {
        key: 'fusion',
        title: 'Fusion',
        order: 30,
        slugs: [SERVICE.fusion],
      },
    ])
    expect(featureMap[SERVICE.render]).toEqual({
      slug: SERVICE.render,
      title: 'Image · 草图成图',
      label: '草图成图',
    })
  })

  it('shows every service while keeping unsupported prompt configuration distinct', () => {
    expect(new Set(Object.keys(featureMap))).toEqual(new Set(SERVICE_SLUGS))
    for (const slug of SERVICE_SLUGS) {
      if (slug === SERVICE.cutout) {
        expect(getFeaturePromptStatus(slug, false)).toBe('unsupported')
        expect(getFeaturePromptStatus(slug, true)).toBe('unsupported')
      } else {
        expect(getFeaturePromptStatus(slug, false)).toBe('unconfigured')
        expect(getFeaturePromptStatus(slug, true)).toBe('configured')
      }
    }
  })
})
