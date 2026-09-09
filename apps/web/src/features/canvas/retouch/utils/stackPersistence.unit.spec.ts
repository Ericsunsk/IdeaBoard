import { describe, expect, it } from 'vitest'
import {
  deserializeRetouchStack,
  serializeRetouchStack,
} from './stackPersistence'
import type { RetouchLayer } from '../types'

describe('retouchStackPersistence', () => {
  it('round-trips effect layers without paint content', () => {
    const layers: RetouchLayer[] = [
      { id: 'source', type: 'source', name: 'Source', visible: true },
      {
        id: 'hsl:1',
        type: 'effect',
        kind: 'hsl',
        name: 'HSL',
        visible: true,
        params: { hue: 10, saturation: 20, lightness: 5 },
      },
      {
        id: 'curves:1',
        type: 'effect',
        kind: 'curves',
        name: 'Curves',
        visible: false,
        params: { channels: {} },
      },
      { id: 'paint:1', type: 'paint', name: 'Paint', visible: true, content: { kind: 'empty' } },
    ]
    const stack = serializeRetouchStack(layers, 'curves:1')
    expect(stack.version).toBe(1)
    expect(stack.layers.map((item) => item.id)).toEqual(['hsl:1', 'curves:1'])
    expect(stack.selectedLayerId).toBe('curves:1')

    const restored = deserializeRetouchStack(stack)
    expect(restored?.layers.map((item) => item.id)).toEqual(['hsl:1', 'curves:1'])
    expect(restored?.selectedLayerId).toBe('curves:1')
    expect(restored?.layers[1]?.visible).toBe(false)
  })

  it('returns null for absent stacks so legacy boards keep working', () => {
    expect(deserializeRetouchStack(null)).toBeNull()
    expect(deserializeRetouchStack(undefined)).toBeNull()
  })

  it('skips invalid entries instead of throwing', () => {
    const restored = deserializeRetouchStack({
      version: 1,
      layers: [{ id: '', kind: 'hsl', name: 'bad', visible: true, params: {} }],
    })
    expect(restored?.layers).toEqual([])
  })
})
