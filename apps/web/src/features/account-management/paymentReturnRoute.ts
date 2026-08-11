export type PaymentReturnSource = 'success' | 'cancel'

export type PaymentReturnContext = {
  publicToken: string
  source: PaymentReturnSource
}

type QueryValue = string | null | undefined | (string | null)[]
export type PaymentReturnQuery = Record<string, QueryValue>

const publicTokenPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

export function parsePaymentReturnQuery(query: PaymentReturnQuery): PaymentReturnContext | null {
  if (query.account !== 'recharge') return null
  if (typeof query.payment_return !== 'string' || !publicTokenPattern.test(query.payment_return)) {
    return null
  }
  if (query.checkout_return !== 'success' && query.checkout_return !== 'cancel') return null

  return {
    publicToken: query.payment_return,
    source: query.checkout_return,
  }
}

export function stripPaymentReturnQuery(query: PaymentReturnQuery): PaymentReturnQuery {
  const cleaned = { ...query }
  delete cleaned.account
  delete cleaned.payment_return
  delete cleaned.checkout_return
  return cleaned
}
