import { describe, expect, it } from 'vitest'
import { parsePaymentReturnQuery, stripPaymentReturnQuery } from './return'

describe('return', () => {
  it('captures a valid recharge return without treating the redirect as payment truth', () => {
    expect(
      parsePaymentReturnQuery({
        account: 'recharge',
        payment_return: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        checkout_return: 'success',
      }),
    ).toEqual({
      publicToken: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
      source: 'success',
    })
  })

  it.each([
    [{ account: 'billing', payment_return: '0198cd29-f457-7e4b-b95e-6cfde41b7222' }],
    [{ account: 'recharge', payment_return: 'short' }],
    [{ account: 'recharge', payment_return: ['0198cd29-f457-7e4b-b95e-6cfde41b7222'] }],
    [
      {
        account: 'recharge',
        payment_return: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        checkout_return: 'paid',
      },
    ],
  ])('ignores malformed or unrelated query %#', (query) => {
    expect(parsePaymentReturnQuery(query)).toBeNull()
  })

  it('removes only payment return keys and preserves the current product route query', () => {
    expect(
      stripPaymentReturnQuery({
        boardId: 'board-8',
        account: 'recharge',
        payment_return: '0198cd29-f457-7e4b-b95e-6cfde41b7222',
        checkout_return: 'cancel',
      }),
    ).toEqual({ boardId: 'board-8' })
  })
})
