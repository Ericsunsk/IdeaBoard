import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, renderWithUser, waitFor } from '@/test/render'
import { createDeferred } from '@/test/deferred'
import StripeSettingsDialog from './StripeSettingsDialog.vue'
import type {
  AdminStripeSettingsResource,
  StripeConfigurationSummary,
} from '../../../session/payment/types'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('../../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

const configuredSummary: StripeConfigurationSummary = {
  isConfigured: true,
  isApiKeyConfigured: true,
  isWebhookSecretConfigured: true,
  accountId: 'acct_123',
  mode: 'test',
  isChargesEnabled: true,
  paymentMethodConfigurationId: 'pmc_ideaboard',
  productTaxCode: 'txcd_10105001',
  isProductReady: true,
  isWebhookReady: true,
  isPurchasesEnabled: true,
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
    expect(screen.getByText('Product Ready')).toBeVisible()
    expect(screen.getByText('Webhook Ready')).toBeVisible()
    expect(screen.getByLabelText('Payment Method Configuration ID')).toHaveValue('pmc_ideaboard')

    const apiKey = screen.getByLabelText('Stripe API Key') as HTMLInputElement
    expect(apiKey.value).toBe('')
    expect(apiKey).toHaveAttribute('type', 'password')
    expect(apiKey).toHaveAttribute('placeholder', '已设置，留空则保持不变')
    expect(screen.getByText('支持 sk_ 或 rk_ 格式。')).toBeVisible()
    expect(screen.queryByDisplayValue('sk_test_secret')).not.toBeInTheDocument()
  })

  it('requires an API Key only before a configuration exists', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({
      ...configuredSummary,
      isConfigured: false,
      isApiKeyConfigured: false,
      isWebhookSecretConfigured: false,
      accountId: undefined,
      mode: undefined,
      isProductReady: false,
      isWebhookReady: false,
    })
    const { user } = renderDialog()

    await screen.findByText('尚未设置')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('请输入 Stripe API Key')).toBeVisible()
    expect(resource.save).not.toHaveBeenCalled()
  })

  it('clears the submitted API Key and closes after a successful atomic save', async () => {
    const { user, emitted } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.clear(screen.getByLabelText('Product Tax Code'))
    await user.type(screen.getByLabelText('Product Tax Code'), 'txcd_10105001')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith({
        apiKey: 'sk_test_secret',
        paymentMethodConfigurationId: 'pmc_ideaboard',
        productTaxCode: 'txcd_10105001',
        isPurchasesEnabled: true,
      }),
    )
    await waitFor(() => expect(emitted()['update:visible']).toEqual([[false]]))
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
    expect(toast.success).toHaveBeenCalledWith('Stripe 设置已保存')
  })

  it('routes save failure to toast error without an inline alert', async () => {
    vi.mocked(resource.save).mockRejectedValueOnce(
      new Error('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key'),
    )
    const { user, emitted } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        '无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key',
      ),
    )
    expect(toast.success).not.toHaveBeenCalled()
    // 精简目标：saveError 内嵌 AdminAlert 已删除，失败一律 toast。
    expect(
      screen.queryByText('无效的 Stripe API Key：请使用 sk_ 或 rk_ 开头的对应环境 Key'),
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(emitted()['update:visible']).toBeUndefined()
    expect((screen.getByLabelText('Stripe API Key') as HTMLInputElement).value).toBe('')
  })

  it('routes load failure to toast error while keeping the retryable status', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('stripe-load-probe'))
    renderDialog()

    // loadError 保留为 frame 状态真相源（可重试 + 禁用保存），同时走 toast。
    expect(await screen.findByText('stripe-load-probe')).toBeVisible()
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('stripe-load-probe'))
    expect(screen.getByRole('button', { name: '重新加载' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
  })

  it('keeps the form interactive during background reloads with a stale summary', async () => {
    const view = renderDialog()
    await screen.findByText('acct_123')

    // 重开触发后台重载：stale summary 就地保留，不闪 spinner、不整表禁用。
    const pendingReload = createDeferred<StripeConfigurationSummary>()
    vi.mocked(resource.get).mockReturnValueOnce(pendingReload.promise)
    await view.rerender({ visible: false, resource })
    await view.rerender({ visible: true, resource })
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(2))

    expect(screen.getByText('acct_123')).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Payment Method Configuration ID')).not.toBeDisabled()
    expect(screen.getByLabelText('Stripe API Key')).not.toBeDisabled()
    expect(screen.getByLabelText('Product Tax Code')).not.toBeDisabled()
    expect(screen.getByRole('switch', { name: '充值开放' })).not.toBeDisabled()
    // 提交仍被保存按钮的 isLoading 禁用兜底，stale 值不会被误提交。
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()

    pendingReload.resolve({ ...configuredSummary, accountId: 'acct_456' })
    expect(await screen.findByText('acct_456')).toBeVisible()
    expect(screen.getByRole('button', { name: '保存' })).not.toBeDisabled()
  })

  it('locks the form only while saving', async () => {
    const pendingSave = createDeferred<StripeConfigurationSummary>()
    vi.mocked(resource.save).mockReturnValueOnce(pendingSave.promise)
    const { user } = renderDialog()
    await screen.findByText('acct_123')

    await user.type(screen.getByLabelText('Stripe API Key'), 'sk_test_secret')
    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(resource.save).toHaveBeenCalled())

    expect(screen.getByLabelText('Payment Method Configuration ID')).toBeDisabled()
    expect(screen.getByLabelText('Stripe API Key')).toBeDisabled()
    expect(screen.getByLabelText('Product Tax Code')).toBeDisabled()
    expect(screen.getByRole('switch', { name: '充值开放' })).toBeDisabled()

    pendingSave.resolve(configuredSummary)
    await waitFor(() => expect(screen.getByLabelText('Stripe API Key')).not.toBeDisabled())
  })
})
