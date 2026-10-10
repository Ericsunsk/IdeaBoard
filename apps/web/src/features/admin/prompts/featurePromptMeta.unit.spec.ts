import { describe, expect, it } from 'vitest'
import { SERVICE, SERVICE_SLUGS } from '../../../core/contracts/imageServices'
import { featureMap, groupedFeatureGroups } from './featurePromptMeta'

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

  it('covers every catalog service except the prompt-ignored cutout', () => {
    // Mirrors the backend prompt boundary (all workflow services except
    // cutout, which carries PromptPolicyIgnored): Admin Prompt reads the
    // shared SERVICE catalog instead of a local slug list.
    const configured = new Set(Object.keys(featureMap))
    for (const slug of SERVICE_SLUGS) {
      if (slug === SERVICE.cutout) {
        expect(configured.has(slug)).toBe(false)
      } else {
        expect(configured.has(slug)).toBe(true)
      }
    }
  })
})
