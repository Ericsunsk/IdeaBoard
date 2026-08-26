import { afterEach, describe, expect, it } from 'vitest'
import { formatIdeaAmount } from './userIdeaCurrency'
import { setUserLocale } from './userLocale'

describe('formatIdeaAmount', () => {
  afterEach(async () => {
    await setUserLocale('en')
  })

  it.each([
    [0, '0 Ideas'],
    [1, '1 Idea'],
    [8, '8 Ideas'],
    [1280, '1,280 Ideas'],
  ])('formats %i credits as %s', (credits, expected) => {
    expect(formatIdeaAmount(credits)).toBe(expected)
  })

  it('uses the active User locale', async () => {
    await setUserLocale('es')
    expect(formatIdeaAmount(1280)).toBe('1280 Ideas')
  })
})
