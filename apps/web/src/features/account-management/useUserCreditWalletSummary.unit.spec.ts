import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserCreditWalletSummary } from './useUserCreditWalletSummary'

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useUserCreditsInvalidationBridge: vi.fn(),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQuery: mocks.useQuery,
}))

vi.mock('./useUserCreditsInvalidationBridge', () => ({
  useUserCreditsInvalidationBridge: mocks.useUserCreditsInvalidationBridge,
}))

vi.mock('./userCreditsResource', () => ({
  fetchUserCreditWallet: vi.fn(),
  userCreditsQueryKeys: { wallet: ['user', 'credits', 'wallet'] },
}))

function mockQuery(options: {
  availableCredits?: number
  isPending?: boolean
  error?: Error | null
}) {
  mocks.useQuery.mockReturnValue({
    data: ref(
      options.availableCredits == null
        ? undefined
        : { available_credits: options.availableCredits },
    ),
    isPending: ref(options.isPending ?? false),
    isError: ref(Boolean(options.error)),
    error: ref(options.error ?? null),
  })
}

describe('useUserCreditWalletSummary', () => {
  beforeEach(() => {
    mocks.useQuery.mockReset()
    mocks.useUserCreditsInvalidationBridge.mockReset()
  })

  it('maps a pending query to loading', () => {
    mockQuery({ isPending: true })

    const summary = useUserCreditWalletSummary()

    expect(summary.state.value).toEqual({ status: 'loading' })
  })

  it('maps wallet data to a ready typed balance', () => {
    mockQuery({ availableCredits: 1280 })

    const summary = useUserCreditWalletSummary()

    expect(summary.state.value).toEqual({ status: 'ready', availableCredits: 1280 })
  })

  it('maps a failed query to error instead of loading', () => {
    const error = new Error('wallet unavailable')
    mockQuery({ error })

    const summary = useUserCreditWalletSummary()

    expect(summary.state.value).toEqual({ status: 'error', error })
    expect(summary.state.value.status).not.toBe('loading')
  })
})
