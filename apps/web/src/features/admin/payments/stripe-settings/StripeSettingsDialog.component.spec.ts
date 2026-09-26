import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, renderWithUser, waitFor } from '../../../../test/render'
import StripeSettingsDialog from './StripeSettingsDialog.vue'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './types'

const toast = vi.hoisted(() => ({ success: vi.fn() }))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({
    confirm: vi.fn().mockResolvedValue(true),
    readReason: () => 'stripe review',
  }),
}))

const configuredSummary: StripeConfigurationSummary = {
  configured: true,
  apiKeyConfigured: true,
  webhookSecretConfigured: true,
  accountId: 'acct_123',
  mode: 'test',
  chargesEnabled: true,
  paymentMethodConfigurationId: 'pmc_ideaboard',
  productTaxCode: 'txcd_10105001',
  productReady: true,
  webhookReady: true,
  purchasesEnabled: true,
  updatedAt: '2026-08-01T00:00:00Z',
}

const resource: AdminStripeSettingsResource = {
  get: vi.fn(),
  save: vi.fn(),
}

function renderDialog(props: { visible?: boolean } = {}) {
  return renderWithUser(StripeSettingsDialog, {
    props: {
      visible: true,
      resource,
      ...props,
    },
    global: {
      plugins: [],
      stubs: { transition: false },
    },
  })
}

describe('StripeSettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(resource.get).mockResolvedValue(configuredSummary)
    vi.mocked(resource.save).mockResolvedValue(configuredSummary)
  })

  it('renders only the safe configuration summary and never rehydrates the API Key', async () => {
    renderDialog()

    expect(await screen.findByText('acct_123')).toBeVisible()
    expect(screen.getByText('Test Mode')).toBeVisible()
    expect(screen.getByText('Product 已就绪')).toBeVisible()
    expect(screen.getByText('Webhook 已就绪')).toBeVisible()
    expect(screen.getByLabelText('Payment Method Configuration ID')).toHaveValue('pmc_ideaboard')

    const apiKey = screen.getByLabelText('Stripe API Key') as HTMLInputElement
    expect(apiKey.value).toBe('')
    expect(apiKey).toHaveAttribute('type', 'password')
    expect(apiKey).toHaveAttribute('placeholder', '已配置，留空则保持不变')
    expect(screen.getByText('支持 sk_ 或 rk_ 格式。')).toBeVisible()
    expect(screen.queryByDisplayValue('sk_test_secret')).not.toBeInTheDocument()
  })

  it('requires an API Key only before a configuration exists', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({
      ...configuredSummary,
      configured: false,
      apiKeyConfigured: false,
      webhookSecretConfigured: false,
      accountId: undefined,
      mode: undefined,
      productReady: false,
      webhookReady: false,
    })
    const { user } = renderDialog()

    await screen.findByText('尚未配置')
    await user.click(screen.getByRole('button', { name: '保存设置' }))

    expect(await screen.findByText('请输入 Stripe API Key')).toBeVisible()
    expect(resource.save).not.toHaveBeenCalled()
  })

  it('clears the submitted API Key and closes after a successful atomic save', async () => {
    const { user, emitted } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.clear(screen.getByLabelText('Product Tax Code'))
    await user.type(screen.getByLabelText('Product Tax Code'), 'txcd_10105001')
    await user.click(screen.getByRole('button', { name: '保存设置' }))

    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith(
        {
          apiKey: 'sk_test_secret',
          paymentMethodConfigurationId: 'pmc_ideaboard',
          productTaxCode: 'txcd_10105001',
          purchasesEnabled: true,
        },
        { operatorReason: 'stripe review' },
      ),
    )
    await waitFor(() => expect(emitted()['update:visible']).toEqual([[false]]))
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
    expect(toast.success).toHaveBeenCalledWith('Stripe 设置已保存')
  })

  it('shows a safe Stripe validation error, keeps the dialog open, and clears the API Key', async () => {
    vi.mocked(resource.save).mockRejectedValueOnce(
      new Error('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key'),
    )
    const { user, emitted } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.click(screen.getByRole('button', { name: '保存设置' }))

    expect(
      await screen.findByText('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key'),
    ).toBeVisible()
    expect(emitted()['update:visible']).toBeUndefined()
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
  })
})
