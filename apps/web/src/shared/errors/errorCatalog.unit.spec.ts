import { describe, expect, it } from 'vitest'
import { readinessErrorCopy, userErrorCode, userFacingErrorCopy } from './errorCatalog'

describe('User-facing error catalog', () => {
  it('extracts stable codes from transport, response data, and causes', () => {
    expect(userErrorCode({ code: 'vision_search_timeout' })).toBe('vision_search_timeout')
    expect(userErrorCode({ data: { error_code: 'insufficient_credits' } })).toBe(
      'insufficient_credits',
    )
    expect(userErrorCode({ cause: { code: 'pricing_unavailable' } })).toBe('pricing_unavailable')
    expect(userErrorCode({ code: 503, data: { code: '503' } })).toBeNull()
  })

  it('maps known codes and never exposes unknown error prose', () => {
    expect(
      userFacingErrorCopy(
        Object.assign(new Error('provider raw detail'), { code: 'vision_search_timeout' }),
        'Fallback',
      ),
    ).toBe('Visual Search took too long. Please try again.')
    expect(userFacingErrorCopy(new Error('provider raw detail'), 'Fallback')).toBe('Fallback')
  })

  it('keeps readiness errors on the same catalog', () => {
    expect(readinessErrorCopy('provider_unavailable')).toBe(
      'This service is unavailable right now. Please try again later.',
    )
  })
})
