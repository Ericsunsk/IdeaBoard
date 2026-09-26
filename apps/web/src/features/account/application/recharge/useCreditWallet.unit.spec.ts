import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCreditWallet } from './useCreditWallet'

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useCreditsSync: vi.fn(),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQuery: mocks.useQuery,
}))

vi.mock('./useCreditsSync', () => ({
  useCreditsSync: mocks.useCreditsSync,
}))

vi.mock('../../infrastructure', () => ({
  fetchCreditWallet: vi.fn(),
  creditsQueryKeys: { wallet: (workspaceId: string) => ['workspace', 'credits', 'wallet', workspaceId] },
}))

vi.mock('../accountWorkspace', () => ({
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    selectionSettled: ref(true),
  }),
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

describe('useCreditWallet', () => {
  beforeEach(() => {
    mocks.useQuery.mockReset()
    mocks.useCreditsSync.mockReset()
  })

  it('maps a pending query to loading', () => {
    mockQuery({ isPending: true })

    const wallet = useCreditWallet()

    expect(wallet.state.value).toEqual({ status: 'loading' })
  })

  it('maps wallet data to a ready typed balance', () => {
    mockQuery({ availableCredits: 1280 })

    const wallet = useCreditWallet()

    expect(wallet.state.value).toEqual({ status: 'ready', availableCredits: 1280 })
  })

  it('maps a failed query to error instead of loading', () => {
    const error = new Error('wallet unavailable')
    mockQuery({ error })

    const wallet = useCreditWallet()

    expect(wallet.state.value).toEqual({ status: 'error', error })
    expect(wallet.state.value.status).not.toBe('loading')
  })
})
