import { describe, expect, it } from 'vitest'
import { generateRewardCode } from './rewardCodeGenerator'

describe('generateRewardCode', () => {
  it('uses the stable 12-character grouped Crockford Base32 format', () => {
    const format = /^[0-9A-HJKMNP-TV-Z]{4}(?:-[0-9A-HJKMNP-TV-Z]{4}){2}$/
    for (let index = 0; index < 50; index += 1) {
      const code = generateRewardCode()
      expect(code).toMatch(format)
      expect(code).not.toMatch(/[ILOU]/)
    }
  })
})
