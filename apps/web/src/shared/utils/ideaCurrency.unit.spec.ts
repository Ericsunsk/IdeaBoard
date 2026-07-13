import { describe, expect, it } from 'vitest'
import { formatIdeaAmount } from './ideaCurrency'

describe('formatIdeaAmount', () => {
  it.each([
    [0, '0 Ideas'],
    [1, '1 Idea'],
    [8, '8 Ideas'],
    [1280, '1,280 Ideas'],
  ])('formats %i credits as %s', (credits, expected) => {
    expect(formatIdeaAmount(credits)).toBe(expected)
  })
})
