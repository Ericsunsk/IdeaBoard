import { afterEach, describe, expect, it, vi } from 'vitest'
import { parsePaymentEvent, subscribeToPaymentEvents } from './paymentsResource'

describe('paymentsResource payment events', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('accepts only the minimal typed payment event contract', () => {
    expect(parsePaymentEvent('{"public_token":"token","status":"credited"}')).toEqual({
      public_token: 'token',
      status: 'credited',
    })
    expect(parsePaymentEvent('{"status":"refund_updated"}')).toEqual({
      status: 'refund_updated',
    })
    expect(parsePaymentEvent('{"status":"dispute_updated"}')).toEqual({
      status: 'dispute_updated',
    })
    expect(parsePaymentEvent('{"status":"unknown"}')).toBeNull()
    expect(parsePaymentEvent('{"public_token":42,"status":"failed"}')).toBeNull()
    expect(parsePaymentEvent('not-json')).toBeNull()
  })

  it('delivers the production SSE event contract and releases the transport', () => {
    const listener = vi.fn()
    let paymentHandler: EventListenerOrEventListenerObject | undefined
    const source = {
      onerror: null as ((event: Event) => void) | null,
      addEventListener: vi.fn((event: string, handler: EventListenerOrEventListenerObject) => {
        if (event === 'payment.updated') paymentHandler = handler
      }),
      removeEventListener: vi.fn(),
      close: vi.fn(),
    }
    const createEventSource = vi.fn(function MockEventSource() {
      return source
    })
    vi.stubGlobal('EventSource', createEventSource)
    const unsubscribe = subscribeToPaymentEvents(listener)

    expect(createEventSource).toHaveBeenCalledWith(expect.stringContaining('/user/events'), {
      withCredentials: true,
    })
    if (typeof paymentHandler !== 'function') throw new Error('payment listener was not registered')
    paymentHandler(
      new MessageEvent('payment.updated', {
        data: '{"public_token":"token","status":"credited"}',
      }),
    )
    expect(listener).toHaveBeenCalledWith({ public_token: 'token', status: 'credited' })

    unsubscribe()
    expect(source.removeEventListener).toHaveBeenCalledWith('payment.updated', paymentHandler)
    expect(source.close).toHaveBeenCalledOnce()
  })
})
