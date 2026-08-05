import { describe, expect, it, vi } from 'vitest'
import { parseUserPaymentEvent, subscribeToUserPaymentEvents } from './userPaymentsResource'

describe('userPaymentsResource payment events', () => {
  it('accepts only the minimal typed payment event contract', () => {
    expect(parseUserPaymentEvent('{"public_token":"token","status":"credited"}')).toEqual({
      public_token: 'token',
      status: 'credited',
    })
    expect(parseUserPaymentEvent('{"status":"refund_updated"}')).toEqual({
      status: 'refund_updated',
    })
    expect(parseUserPaymentEvent('{"status":"dispute_updated"}')).toEqual({
      status: 'dispute_updated',
    })
    expect(parseUserPaymentEvent('{"status":"unknown"}')).toBeNull()
    expect(parseUserPaymentEvent('{"public_token":42,"status":"failed"}')).toBeNull()
    expect(parseUserPaymentEvent('not-json')).toBeNull()
  })

  it('delivers the production SSE event contract and releases the transport', () => {
    const listener = vi.fn()
    let paymentHandler: EventListenerOrEventListenerObject | undefined
    const source = {
      addEventListener: vi.fn((event: string, handler: EventListenerOrEventListenerObject) => {
        if (event === 'payment') paymentHandler = handler
      }),
      removeEventListener: vi.fn(),
      close: vi.fn(),
    }
    const createEventSource = vi.fn(() => source)
    const unsubscribe = subscribeToUserPaymentEvents(listener, createEventSource)

    expect(createEventSource).toHaveBeenCalledWith(expect.stringContaining('/user/payments/events'), {
      withCredentials: true,
    })
    if (typeof paymentHandler !== 'function') throw new Error('payment listener was not registered')
    paymentHandler(
      new MessageEvent('payment', {
        data: '{"public_token":"token","status":"credited"}',
      }),
    )
    expect(listener).toHaveBeenCalledWith({ public_token: 'token', status: 'credited' })

    unsubscribe()
    expect(source.removeEventListener).toHaveBeenCalledWith('payment', paymentHandler)
    expect(source.close).toHaveBeenCalledOnce()
  })
})
