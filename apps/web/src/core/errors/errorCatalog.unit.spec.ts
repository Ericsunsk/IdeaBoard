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
    ).toBe('Visual Search failed. Please try again.')
    expect(userFacingErrorCopy(new Error('provider raw detail'), 'Fallback')).toBe('Fallback')
  })

  it('keeps readiness errors on the same catalog', () => {
    expect(readinessErrorCopy('provider_unavailable')).toBe(
      'This service is unavailable right now. Please try again later.',
    )
  })

  it('resolves feature guidance codes only through a caller-supplied overlay', () => {
    const guidance = Object.assign(new Error('Select a workspace'), {
      code: 'workspace_selection_required',
    })
    expect(userFacingErrorCopy(guidance, 'Fallback')).toBe('Fallback')
    expect(
      userFacingErrorCopy(guidance, 'Fallback', {
        workspace_selection_required: 'account.errors.workspaceRequired',
      }),
    ).toBe('Select a workspace to view billing information.')
  })

  it('lets the caller overlay take precedence over built-in codes', () => {
    const error = Object.assign(new Error('provider raw detail'), {
      code: 'vision_search_timeout',
    })
    expect(userFacingErrorCopy(error, 'Fallback')).toBe('Visual Search failed. Please try again.')
    expect(
      userFacingErrorCopy(error, 'Fallback', {
        vision_search_timeout: 'account.errors.workspaceRequired',
      }),
    ).toBe('Select a workspace to view billing information.')
  })
})
