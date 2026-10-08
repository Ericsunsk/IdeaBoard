import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AdminMailConfigCard from './AdminMailConfigCard.vue'
import { ADMIN_PROBE_OPERATOR_REASON } from '../shared/lib/confirmSettingsSave'
import type { AdminMailConfigResource } from './mailConfigResource'
import type { MailConfigSummary } from './mailConfigTypes'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('test reason'))

vi.mock('../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

const summary: MailConfigSummary = {
  configured: true,
  api_key_configured: true,
  from_name: 'IdeaBoard',
  from_email: 'noreply@example.com',
  reply_to: 'support@example.com',
  config_version: 2,
  readiness: 'ready',
  updated_at: '2026-08-05T00:00:00Z',
  last_tested_at: '2026-08-05T00:01:00Z',
  last_test_error_code: null,
}

const resource: AdminMailConfigResource = {
  get: vi.fn(),
  update: vi.fn(),
  clear: vi.fn(),
  test: vi.fn(),
}

function renderCard() {
  return renderWithUser(AdminMailConfigCard, {
    props: { resource },
    global: { plugins: [], stubs: { transition: false } },
  })
}

describe('AdminMailConfigCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReturnValue('test reason')
    vi.mocked(resource.get).mockResolvedValue(summary)
    vi.mocked(resource.update).mockResolvedValue({ ...summary, readiness: 'not_tested' })
    vi.mocked(resource.clear).mockResolvedValue(undefined)
    vi.mocked(resource.test).mockResolvedValue(undefined)
  })

  it('renders only the safe configuration metadata and never hydrates the API Key', async () => {
    renderCard()

    expect(await screen.findByText('Ready')).toBeVisible()
    expect(screen.getByDisplayValue('IdeaBoard')).toBeVisible()
    expect(screen.getByDisplayValue('noreply@example.com')).toBeVisible()
    const apiKey = screen.getByLabelText('Resend API Key') as HTMLInputElement
    expect(apiKey.value).toBe('')
    expect(apiKey).toHaveAttribute('type', 'password')
    expect(screen.queryByDisplayValue('re_secret')).not.toBeInTheDocument()
  })

  it('requires an API Key for the first configuration', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({
      ...summary,
      configured: false,
      api_key_configured: false,
      readiness: 'not_configured',
    })
    const { user } = renderCard()

    await screen.findByText('未配置')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('请输入 Resend API Key')).toBeVisible()
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('keeps an existing API Key when saving sender settings and clears the field afterwards', async () => {
    const { user } = renderCard()
    await screen.findByText('Ready')

    const fromName = screen.getByRole('textbox', { name: /发件人名称/ })
    await user.clear(fromName)
    await user.type(fromName, 'IdeaBoard Ops')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(resource.update).toHaveBeenCalledWith(
        {
          from_name: 'IdeaBoard Ops',
          from_email: 'noreply@example.com',
          reply_to: 'support@example.com',
        },
        { operatorReason: 'test reason' },
      ),
    )
    expect(confirmMock).toHaveBeenCalledOnce()
    expect((screen.getByLabelText('Resend API Key') as HTMLInputElement).value).toBe('')
    expect(toast.success).toHaveBeenCalledWith('邮件配置已保存，请发送 Test Email')
  })

  it('aborts the save when the operator cancels the confirm dialog', async () => {
    confirmMock.mockResolvedValueOnce(false)
    const { user } = renderCard()
    await screen.findByText('Ready')

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(confirmMock).toHaveBeenCalledOnce())
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('sends a Test Email without a confirm dialog and reloads the readiness metadata', async () => {
    const { user } = renderCard()
    await screen.findByText('Ready')

    await user.type(screen.getByLabelText('测试收件地址'), 'ops@example.com')
    await user.click(screen.getByRole('button', { name: '发送 Test Email' }))

    await waitFor(() =>
      expect(resource.test).toHaveBeenCalledWith('ops@example.com', {
        operatorReason: ADMIN_PROBE_OPERATOR_REASON,
      }),
    )
    expect(confirmMock).not.toHaveBeenCalled()
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(2))
    expect(toast.success).toHaveBeenCalledWith('Test Email 已发送')
  })
})
