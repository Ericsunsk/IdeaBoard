import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import WorkspaceBudgetSection from './WorkspaceBudgetSection.vue'
import type { WorkspaceBudgetSummary } from '../../session/budget/resource'

vi.mock('../../../../core/composables/useAppToast')

const budgetTransport = vi.hoisted(() => ({
  fetch: vi.fn(),
  update: vi.fn(),
}))

vi.mock('../../session/budget/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../session/budget/resource')>()
  return {
    ...actual,
    fetchWorkspaceBudget: budgetTransport.fetch,
    updateWorkspaceBudget: budgetTransport.update,
  }
})

function monthlyBudget(overrides: Partial<WorkspaceBudgetSummary> = {}): WorkspaceBudgetSummary {
  return {
    workspaceId: 'ws_123',
    billingAccountId: 'ba_1',
    period: 'monthly',
    limitCredits: 1000,
    usedCredits: 250,
    remainingCredits: 750,
    periodStart: '2026-10-01T00:00:00Z',
    periodEnd: '2026-11-01T00:00:00Z',
    ...overrides,
  }
}

function renderSection(consumptionScope: string | null = 'member') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(WorkspaceBudgetSection, {
    props: { workspaceId: 'ws_123', consumptionScope, isExpanded: true },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('WorkspaceBudgetSection', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
  })

  test('renders monthly usage with remaining for members', async () => {
    budgetTransport.fetch.mockResolvedValue(monthlyBudget())
    renderSection('member')

    expect(await screen.findByText('250 / 1000 Ideas used')).toBeVisible()
    expect(screen.getByText('750 Ideas left')).toBeVisible()
    expect(screen.getByText('Consumption budget')).toBeVisible()
    // Members see the read-only note instead of management controls.
    expect(screen.getByText(/Only billing managers can change the budget/)).toBeVisible()
    expect(screen.queryByLabelText('Monthly limit (Ideas)')).toBeNull()
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('warns when the cap is reached', async () => {
    budgetTransport.fetch.mockResolvedValue(monthlyBudget({ usedCredits: 1000, remainingCredits: 0 }))
    renderSection('member')

    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(/Budget exhausted/)
  })

  test('billing managers can set a monthly limit', async () => {
    budgetTransport.fetch.mockResolvedValue(monthlyBudget())
    budgetTransport.update.mockImplementation(async (_id: string) => ({
      ...monthlyBudget(),
      limitCredits: 500,
      usedCredits: 250,
      remainingCredits: 250,
    }))
    const view = renderSection('account')

    const input = (await screen.findByLabelText('Monthly limit (Ideas)')) as HTMLInputElement
    await view.user.clear(input)
    await view.user.type(input, '500')
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(budgetTransport.update).toHaveBeenCalledWith('ws_123', {
        period: 'monthly',
        limitCredits: 500,
      }),
    )
    expect(toast.success).toHaveBeenCalledWith('Budget updated.')
  })

  test('rejects a non-numeric limit before transport', async () => {
    budgetTransport.fetch.mockResolvedValue(monthlyBudget())
    const view = renderSection('account')

    const input = (await screen.findByLabelText('Monthly limit (Ideas)')) as HTMLInputElement
    await view.user.clear(input)
    await view.user.type(input, 'abc')
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/positive whole number/)
    expect(budgetTransport.update).not.toHaveBeenCalled()
  })

  test('renders unlimited usage without a cap', async () => {
    budgetTransport.fetch.mockResolvedValue({
      workspaceId: 'ws_123',
      billingAccountId: 'ba_1',
      period: 'unlimited',
      limitCredits: null,
      usedCredits: 40,
      remainingCredits: null,
      periodStart: null,
      periodEnd: null,
    })
    renderSection('account')

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByText(/Ideas left/)).toBeNull()
  })

  test('retries after a load failure', async () => {
    budgetTransport.fetch
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(monthlyBudget())
    const view = renderSection('member')

    expect(await screen.findByText('Could not load the budget.')).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('250 / 1000 Ideas used')).toBeVisible()
  })
})
