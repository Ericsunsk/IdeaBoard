import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import DefaultMemberQuotaSection from './DefaultMemberQuotaSection.vue'
import type { DefaultMemberQuotaSummary } from '../../session/quota/resource'

vi.mock('../../../../core/composables/useAppToast')

const defaultTransport = vi.hoisted(() => ({
  fetch: vi.fn(),
  update: vi.fn(),
}))

vi.mock('../../session/quota/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../session/quota/resource')>()
  return {
    ...actual,
    fetchDefaultMemberQuota: defaultTransport.fetch,
    updateDefaultMemberQuota: defaultTransport.update,
  }
})

function monthlyDefault(
  overrides: Partial<DefaultMemberQuotaSummary> = {},
): DefaultMemberQuotaSummary {
  return {
    billingAccountId: 'ba_1',
    workspaceId: 'ws_123',
    period: 'monthly',
    limitCredits: 1000,
    ...overrides,
  }
}

function renderSection(consumptionScope: string | null = 'member') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(DefaultMemberQuotaSection, {
    props: { workspaceId: 'ws_123', consumptionScope, isExpanded: true },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('DefaultMemberQuotaSection', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
  })

  test('renders the configured default for members', async () => {
    defaultTransport.fetch.mockResolvedValue(monthlyDefault())
    renderSection('member')

    expect(await screen.findByText('New members get 1000 Ideas')).toBeVisible()
    expect(screen.getByText('Default quota for new members')).toBeVisible()
    expect(screen.getByText(/Only billing managers can set the default/)).toBeVisible()
    expect(screen.queryByLabelText('Period')).toBeNull()
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('guides members when no default is configured', async () => {
    defaultTransport.fetch.mockRejectedValue({ code: 'default_member_quota_not_configured' })
    renderSection('member')

    expect(await screen.findByRole('alert')).toHaveTextContent(/No default quota yet/)
  })

  test('billing managers see presets and the period editor', async () => {
    defaultTransport.fetch.mockResolvedValue(monthlyDefault())
    renderSection('account')

    expect(await screen.findByText('New members get 1000 Ideas')).toBeVisible()
    expect(screen.getByRole('button', { name: '200 / month' })).toBeVisible()
    expect(screen.getByRole('button', { name: '1000 / month' })).toBeVisible()
    expect(screen.getByRole('button', { name: '2000 / month' })).toBeVisible()
    expect(screen.queryByText(/Only billing managers can set the default/)).toBeNull()
  })

  test('billing managers can save the default with a preset', async () => {
    defaultTransport.fetch.mockResolvedValue(monthlyDefault())
    defaultTransport.update.mockResolvedValue(monthlyDefault({ limitCredits: 200 }))
    const view = renderSection('account')

    await view.user.click(await screen.findByRole('button', { name: '200 / month' }))
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(defaultTransport.update).toHaveBeenCalledWith('ws_123', {
        period: 'monthly',
        limitCredits: 200,
      }),
    )
    expect(toast.success).toHaveBeenCalledWith('Default quota updated.')
  })

  test('retries after a load failure', async () => {
    defaultTransport.fetch
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(monthlyDefault())
    const view = renderSection('member')

    expect(await screen.findByText('Could not load the default quota.')).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('New members get 1000 Ideas')).toBeVisible()
  })
})
