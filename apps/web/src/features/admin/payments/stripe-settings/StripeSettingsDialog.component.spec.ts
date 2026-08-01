import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, renderWithUser, waitFor } from '../../../../test/render'
import { primeVueTestOptions } from '../../../../test/primeVue'
import StripeSettingsDialog from './StripeSettingsDialog.vue'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './stripeSettingsTypes'

const toast = vi.hoisted(() => ({ success: vi.fn() }))

vi.mock('../../shared/lib/adminToast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/adminToast')>()),
  useAdminToast: () => toast,
}))

const configuredSummary: StripeConfigurationSummary = {
  configured: true,
  apiKeyConfigured: true,
  webhookSecretConfigured: true,
  accountId: 'acct_123',
  mode: 'test',
  chargesEnabled: true,
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
      plugins: [[PrimeVue, primeVueTestOptions]],
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

    const apiKey = screen.getByLabelText('Stripe API Key') as HTMLInputElement
    expect(apiKey.value).toBe('')
    expect(apiKey).toHaveAttribute('type', 'password')
    expect(apiKey).toHaveAttribute('placeholder', '已配置，留空则保持不变')
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
      expect(resource.save).toHaveBeenCalledWith({
        apiKey: 'sk_test_secret',
        productTaxCode: 'txcd_10105001',
        purchasesEnabled: true,
      }),
    )
    await waitFor(() => expect(emitted()['update:visible']).toEqual([[false]]))
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
    expect(toast.success).toHaveBeenCalledWith('Stripe 设置已保存')
  })

  it('keeps the dialog open and clears the API Key when saving fails', async () => {
    vi.mocked(resource.save).mockRejectedValueOnce(new Error('Stripe 无法验证此 API Key'))
    const { user, emitted } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.click(screen.getByRole('button', { name: '保存设置' }))

    expect(await screen.findByText('保存 Stripe 设置失败，请重试')).toBeVisible()
    expect(emitted()['update:visible']).toBeUndefined()
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
  })
})
