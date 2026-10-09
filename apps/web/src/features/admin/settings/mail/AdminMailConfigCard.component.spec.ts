import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import AdminMailConfigCard from './AdminMailConfigCard.vue'
import { ADMIN_PROBE_OPERATOR_REASON } from '../../shared/lib/confirmSettingsSave'
import type { AdminMailConfigResource } from './resource'
import type { MailConfigSummary } from './types'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('test reason'))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

const summary: MailConfigSummary = {
  configured: true,
  api_key_configured: true,
  enabled: true,
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
    vi.mocked(resource.update).mockImplementation(async (input) => ({
      ...summary,
      ...input,
      readiness: 'not_tested',
    }))
    vi.mocked(resource.clear).mockResolvedValue(undefined)
    vi.mocked(resource.test).mockResolvedValue(undefined)
  })

  it('renders only the safe configuration metadata and never hydrates the API Key', async () => {
    renderCard()

    expect(await screen.findByText('Ready')).toBeVisible()
    expect(screen.getByDisplayValue('IdeaBoard')).toBeVisible()
    expect(screen.getByDisplayValue('noreply@example.com')).toBeVisible()
    const apiKey = screen.getByLabelText('邮件服务商 API Key') as HTMLInputElement
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

    expect(await screen.findByText('请输入邮件服务商 API Key')).toBeVisible()
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
          enabled: true,
        },
        { operatorReason: 'test reason' },
      ),
    )
    expect(confirmMock).toHaveBeenCalledOnce()
    expect((screen.getByLabelText('邮件服务商 API Key') as HTMLInputElement).value).toBe('')
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

  it('renders the enabled switch checked from the summary', async () => {
    renderCard()
    await screen.findByText('Ready')

    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })
    expect(toggle).toBeChecked()
    expect(toggle).toBeEnabled()
  })

  it('hydrates the switch unchecked when the summary is disabled', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({ ...summary, enabled: false })
    renderCard()
    await screen.findByText('Ready')

    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })
    expect(toggle).not.toBeChecked()
  })

  it('toggles enabled off through the save channel with a confirm', async () => {
    const { user } = renderCard()
    await screen.findByText('Ready')
    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })

    await user.click(toggle)

    await waitFor(() =>
      expect(resource.update).toHaveBeenCalledWith(
        {
          from_name: 'IdeaBoard',
          from_email: 'noreply@example.com',
          reply_to: 'support@example.com',
          enabled: false,
        },
        { operatorReason: 'test reason' },
      ),
    )
    expect(confirmMock).toHaveBeenCalledOnce()
    await waitFor(() => expect(toggle).not.toBeChecked())
    expect(toast.success).toHaveBeenCalledWith('邮件配置已保存，请发送 Test Email')
  })

  it.each([true, false])(
    'toggles enabled from %s without saving or discarding invalid sender and API Key drafts',
    async (initialEnabled) => {
      vi.mocked(resource.get).mockResolvedValueOnce({ ...summary, enabled: initialEnabled })
      const { user } = renderCard()
      await screen.findByText('Ready')

      const fromName = screen.getByRole('textbox', { name: /发件人名称/ })
      const fromEmail = screen.getByRole('textbox', { name: '发件人 Email' })
      const replyTo = screen.getByRole('textbox', { name: 'Reply-To Email' })
      const apiKey = screen.getByLabelText('邮件服务商 API Key')
      await user.clear(fromName)
      await user.type(fromName, 'Draft Sender')
      await user.clear(fromEmail)
      await user.type(fromEmail, 'invalid-draft')
      await user.clear(replyTo)
      await user.type(replyTo, 'draft-support@example.com')
      await user.type(apiKey, 'draft-api-key')

      const toggle = screen.getByRole('switch', { name: '启用邮件发送' })
      await user.click(toggle)

      await waitFor(() =>
        expect(resource.update).toHaveBeenCalledWith(
          {
            from_name: summary.from_name,
            from_email: summary.from_email,
            reply_to: summary.reply_to,
            enabled: !initialEnabled,
          },
          { operatorReason: 'test reason' },
        ),
      )
      await waitFor(() => expect(toggle).toBeEnabled())
      expect(toggle).toHaveAttribute('aria-checked', String(!initialEnabled))
      expect(fromName).toHaveValue('Draft Sender')
      expect(fromEmail).toHaveValue('invalid-draft')
      expect(replyTo).toHaveValue('draft-support@example.com')
      expect(apiKey).toHaveValue('draft-api-key')

      await user.clear(fromEmail)
      await user.type(fromEmail, 'draft@example.com')
      await user.click(screen.getByRole('button', { name: '保存' }))

      await waitFor(() =>
        expect(resource.update).toHaveBeenLastCalledWith(
          {
            from_name: 'Draft Sender',
            from_email: 'draft@example.com',
            reply_to: 'draft-support@example.com',
            api_key: 'draft-api-key',
            enabled: !initialEnabled,
          },
          { operatorReason: 'test reason' },
        ),
      )
      await waitFor(() => expect(apiKey).toHaveValue(''))
    },
  )

  it('rolls back the switch when the toggle confirm is cancelled', async () => {
    confirmMock.mockResolvedValueOnce(false)
    const { user } = renderCard()
    await screen.findByText('Ready')
    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })

    await user.click(toggle)

    await waitFor(() => expect(confirmMock).toHaveBeenCalledOnce())
    expect(resource.update).not.toHaveBeenCalled()
    await waitFor(() => expect(toggle).toBeChecked())
  })

  it('rolls back the switch when the toggle save fails', async () => {
    vi.mocked(resource.update).mockRejectedValueOnce(new Error('boom'))
    const { user } = renderCard()
    await screen.findByText('Ready')
    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })

    await user.click(toggle)

    await waitFor(() => expect(resource.update).toHaveBeenCalledOnce())
    await waitFor(() => expect(toggle).toBeChecked())
    expect(toast.error).toHaveBeenCalled()
  })

  it('keeps the delete action inside the danger zone and clears with a destructive confirm', async () => {
    const { user } = renderCard()
    await screen.findByText('Ready')

    const dangerZone = screen.getByRole('group', { name: 'Danger Zone' })
    expect(dangerZone).toHaveTextContent('删除邮件发送配置后事务邮件将不可用，且不可撤销。')
    const deleteButton = within(dangerZone).getByRole('button', { name: '删除配置' })
    await user.click(deleteButton)

    await waitFor(() =>
      expect(resource.clear).toHaveBeenCalledWith({ operatorReason: 'test reason' }),
    )
    expect(confirmMock).toHaveBeenCalledOnce()
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(2))
    expect(toast.success).toHaveBeenCalledWith('邮件配置已删除')
  })
})
