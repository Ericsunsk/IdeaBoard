import { describe, expect, it } from 'vitest'
import {
  RETOUCH_EFFECT_RAIL,
  RETOUCH_KIND_ICONS,
  RETOUCH_UNKNOWN_KIND_ICON,
  resolveRetouchChromeSelection,
  resolveRetouchKindIcon,
  resolveRetouchLayerIcon,
  resolveRetouchLayerKindKey,
} from './retouchViewModel'
import { RETOUCH_EFFECT_RAIL_ORDER, RETOUCH_KIND_LABEL_KEYS } from '../core/kindLabels.ts'
import { IMAGE_RETOUCH_DEFINITIONS } from '../core/definitions.ts'
import { createDefaultImageEffectLayer } from '../core/layers/layerStack.ts'
import type { ImageRetouchKind, RetouchLayer } from '../core/model.ts'

/*
 * Thin view-model contract (S8): kind -> icon/label/rail derives from the
 * definition key set, labels stay as labelKeys for call-time t(), unknown
 * kinds degrade to the generic thumb, and Chrome selection resolves rail +
 * list cursor in one place.
 */
describe('retouchViewModel', () => {
  const definitionKinds = Object.keys(IMAGE_RETOUCH_DEFINITIONS) as ImageRetouchKind[]

  it('covers exactly the definition kinds in the legacy rail order', () => {
    expect([...RETOUCH_EFFECT_RAIL_ORDER]).toEqual([
      'threshold',
      'hsl',
      'sharpen',
      'curves',
      'levels',
    ])
    expect(RETOUCH_EFFECT_RAIL.map((item) => item.kind).sort()).toEqual([...definitionKinds].sort())
  })

  it('keeps every kind label as a call-time labelKey with a rail icon', () => {
    for (const kind of definitionKinds) {
      expect(RETOUCH_KIND_LABEL_KEYS[kind]).toBe(`canvas.retouch.effects.${kind}`)
      expect(RETOUCH_KIND_ICONS[kind]).toBeDefined()
      expect(resolveRetouchKindIcon(kind)).toBe(RETOUCH_KIND_ICONS[kind])
    }
    expect(RETOUCH_EFFECT_RAIL).toHaveLength(definitionKinds.length)
    for (const item of RETOUCH_EFFECT_RAIL) {
      expect(item.labelKey).toBe(`canvas.retouch.effects.${item.kind}`)
      expect(item.icon).toBe(RETOUCH_KIND_ICONS[item.kind])
    }
  })

  it('resolves layer thumbs per type and degrades unknown kinds without throwing', () => {
    const source: RetouchLayer = { id: 'source', type: 'source', name: 'Original', visible: true }
    expect(resolveRetouchLayerKindKey(source)).toBe('source')

    const hsl = createDefaultImageEffectLayer('hsl')
    expect(resolveRetouchLayerKindKey(hsl)).toBe('hsl')
    expect(resolveRetouchLayerIcon(hsl)).toBe(RETOUCH_KIND_ICONS.hsl)

    const unknownEffect = {
      id: 'mystery',
      type: 'effect',
      kind: 'sepia',
      name: 'Mystery',
      visible: true,
      params: {},
    } as unknown as RetouchLayer
    expect(() => resolveRetouchLayerIcon(unknownEffect)).not.toThrow()
    expect(resolveRetouchLayerIcon(unknownEffect)).toBe(RETOUCH_UNKNOWN_KIND_ICON)
    expect(resolveRetouchKindIcon('sepia' as ImageRetouchKind)).toBe(RETOUCH_UNKNOWN_KIND_ICON)
  })

  it('resolves Chrome rail and list selection from one editing identity', () => {
    const pending = createDefaultImageEffectLayer('threshold')
    // Pending preview: rail follows the staged kind while the list cursor
    // parks on source.
    expect(
      resolveRetouchChromeSelection({ selectedEffectLayer: pending, selectedLayerId: 'source' }),
    ).toEqual({ selectedKind: 'threshold', selectedLayerId: 'source' })

    expect(
      resolveRetouchChromeSelection({ selectedEffectLayer: null, selectedLayerId: 'source' }),
    ).toEqual({ selectedKind: null, selectedLayerId: 'source' })
  })
})
