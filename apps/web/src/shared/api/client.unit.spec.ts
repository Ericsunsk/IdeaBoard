import { describe, expect, it } from 'vitest'
import { createApiError } from './client'

describe('createApiError', () => {
  it('preserves a string machine code from a structured HTTP error', () => {
    expect(
      createApiError({ code: 'insufficient_credits', msg: 'Idea不足', message_safe: true }, 402),
    ).toMatchObject({
      code: 'insufficient_credits',
      status: 402,
      message: 'Idea不足',
    })
  })

  it('preserves the pricing-unavailable machine code from a structured HTTP error', () => {
    expect(
      createApiError(
        { code: 'pricing_unavailable', msg: '暂时无法获取报价，请重试', message_safe: true },
        503,
      ),
    ).toMatchObject({
      code: 'pricing_unavailable',
      status: 503,
      message: '暂时无法获取报价，请重试',
    })
  })

  it('replaces sensitive and internal transport messages while preserving error metadata', () => {
    expect(
      createApiError({ code: 503, msg: 'dial tcp 127.0.0.1:5432 with api_key=secret' }, 503),
    ).toMatchObject({
      code: 503,
      status: 503,
      hasSafeDisplayMessage: false,
      message: 'The request could not be completed.',
      msg: '',
    })
  })
})
