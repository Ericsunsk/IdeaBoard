import { describe, expect, it } from 'vitest'
import { resolveFetchUrl } from './flowMediaIO'

describe('flow media URL resolution', () => {
  it('uses the same-origin upload proxy for local API upload URLs', () => {
    expect(
      resolveFetchUrl('http://localhost:6000/uploads/canvas/U123/board/content/image.png'),
    ).toBe('http://localhost:3000/uploads/canvas/U123/board/content/image.png')
  })
})
