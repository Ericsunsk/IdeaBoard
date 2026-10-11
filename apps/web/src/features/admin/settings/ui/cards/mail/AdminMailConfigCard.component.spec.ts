import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../../../test/render'
import AdminMailConfigCard from './AdminMailConfigCard.vue'
import type { AdminMailConfigResource } from '../../../session/mail/resource'
import type { MailConfigSummary } from '../../../session/mail/types'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('test reason'))

vi.mock('../../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../../../../shared/session/feedback/useAdminConfirm', () => ({
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

    // 健康三态（方案 A）：ready→Ready success，载体统一 AdminStatusPill，无英文孤岛。
    const ready = await screen.findByText('Ready')
    expect(ready).toBeVisible()
    expect(ready.parentElement?.className ?? '').toContain('bg-success-soft')
    expect(screen.queryByText('Normal')).not.toBeInTheDocument()
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

    // 未配置走占位/空态语义：Status 统一为未验证 neutral，不再用“未配置”胶囊。
    const unverified = await screen.findByText('Unverified')
    expect(unverified).toBeVisible()
    expect(unverified.parentElement?.className ?? '').toContain('bg-muted')
    expect(screen.queryByText('未配置')).not.toBeInTheDocument()
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
        { operatorReason: '' },
      ),
    )
    expect(confirmMock).not.toHaveBeenCalled()
    expect((screen.getByLabelText('邮件服务商 API Key') as HTMLInputElement).value).toBe('')
    expect(toast.success).toHaveBeenCalledWith('邮件配置已保存，请发送 Test Email')
  })

  it('sends a Test Email without a confirm dialog and reloads the readiness metadata', async () => {
    const { user } = renderCard()
    await screen.findByText('Ready')

    await user.type(screen.getByLabelText('测试收件地址'), 'ops@example.com')
    await user.click(screen.getByRole('button', { name: '发送 Test Email' }))

    await waitFor(() =>
      expect(resource.test).toHaveBeenCalledWith('ops@example.com', {
        operatorReason: '',
      }),
    )
    expect(confirmMock).not.toHaveBeenCalled()
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(2))
    expect(toast.success).toHaveBeenCalledWith('Test Email 已发送')
  })

  it('shows the testing spinner in the test recipient input suffix instead of the test button', async () => {
    let resolveTest!: (value: undefined) => void
    vi.mocked(resource.test).mockReturnValueOnce(
      new Promise<undefined>((resolve) => {
        resolveTest = resolve
      }),
    )
    const { user } = renderCard()
    await screen.findByText('Ready')

    await user.type(screen.getByLabelText('测试收件地址'), 'ops@example.com')
    const testButton = screen.getByRole('button', { name: '发送 Test Email' })
    await user.click(testButton)

    // 输入栏尾部出现可访问的 testing 状态，按钮保持 disabled 但自身无 spinner。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('Testing…')
    const spinner = status.querySelector('svg')
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    expect(testButton).toBeDisabled()
    expect(testButton.querySelector('svg')).not.toBeInTheDocument()
    // loading 行为：输入栏标记 aria-busy（不测内部 Spinner 实现）。
    expect(screen.getByLabelText('测试收件地址')).toHaveAttribute('aria-busy', 'true')

    resolveTest(undefined)
    await waitFor(() => {
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })
    expect(screen.getByLabelText('测试收件地址')).not.toHaveAttribute('aria-busy')
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

  it('toggles enabled off through the save channel without a confirm', async () => {
    let resolveUpdate!: (value: unknown) => void
    vi.mocked(resource.update).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveUpdate = resolve as (value: unknown) => void
      }) as Promise<MailConfigSummary>,
    )
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
        { operatorReason: '' },
      ),
    )
    expect(confirmMock).not.toHaveBeenCalled()
    // 乐观 intent 同 tick 翻转：同一节点即时 unchecked + disabled。
    const pending = screen.getByRole('switch', { name: '启用邮件发送' })
    expect(pending).toBe(toggle)
    expect(pending).not.toBeChecked()
    expect(pending).toBeDisabled()

    resolveUpdate({ ...summary, enabled: false, readiness: 'not_tested' })
    await waitFor(() => expect(toggle).not.toBeChecked())
    await waitFor(() => expect(toggle).toBeEnabled())
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
          { operatorReason: '' },
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
          { operatorReason: '' },
        ),
      )
      await waitFor(() => expect(apiKey).toHaveValue(''))
    },
  )

  it('rolls back the switch when the toggle save fails', async () => {
    let rejectUpdate!: (error: Error) => void
    vi.mocked(resource.update).mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectUpdate = reject
      }) as Promise<MailConfigSummary>,
    )
    const { user } = renderCard()
    await screen.findByText('Ready')
    const toggle = await screen.findByRole('switch', { name: '启用邮件发送' })
    await user.click(toggle)

    await waitFor(() => expect(resource.update).toHaveBeenCalledOnce())
    // 乐观先翻为 unchecked + disabled，失败回滚同一节点。
    const pending = screen.getByRole('switch', { name: '启用邮件发送' })
    expect(pending).toBe(toggle)
    expect(pending).not.toBeChecked()
    expect(pending).toBeDisabled()

    rejectUpdate(new Error('boom'))
    await waitFor(() => expect(toggle).toBeChecked())
    await waitFor(() => expect(toggle).toBeEnabled())
    expect(toast.error).toHaveBeenCalled()
  })

  it('routes load failure to toast error without a Status pill or inline alert', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('toast-probe-load'))
    renderCard()

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-load')),
    )
    // 加载失败不进 Status：无Ready/未验证/异常胶囊，也无加载失败/加载中文案。
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Unverified')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    expect(screen.queryByText('加载失败')).not.toBeInTheDocument()
    expect(screen.queryByText('加载中')).not.toBeInTheDocument()
    expect(screen.queryByText('Normal')).not.toBeInTheDocument()
    expect(toast.success).not.toHaveBeenCalled()
    expect(screen.queryByText(/toast-probe-load/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('routes test failure to toast error without an inline alert', async () => {
    vi.mocked(resource.test).mockRejectedValueOnce(new Error('toast-probe-test'))
    const { user } = renderCard()
    await screen.findByText('Ready')

    await user.type(screen.getByLabelText('测试收件地址'), 'ops@example.com')
    await user.click(screen.getByRole('button', { name: '发送 Test Email' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-test')),
    )
    expect(toast.success).not.toHaveBeenCalled()
    expect(screen.queryByText(/toast-probe-test/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
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

  it('maps not_tested to unverified neutral without warning yellow', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({ ...summary, readiness: 'not_tested' })
    renderCard()

    const unverified = await screen.findByText('Unverified')
    expect(unverified).toBeVisible()
    expect(unverified.parentElement?.className ?? '').toContain('bg-muted')
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    expect(screen.queryByText('Normal')).not.toBeInTheDocument()
  })

  it('maps unavailable to abnormal danger', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({ ...summary, readiness: 'unavailable' })
    renderCard()

    const abnormal = await screen.findByText('Error')
    expect(abnormal).toBeVisible()
    expect(abnormal.parentElement?.className ?? '').toContain('bg-destructive-soft')
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Unverified')).not.toBeInTheDocument()
  })

  it('shows a spinner without a Status pill while loading', async () => {
    let resolveGet: ((value: MailConfigSummary) => void) | undefined
    vi.mocked(resource.get).mockImplementationOnce(
      () =>
        new Promise<MailConfigSummary>((resolve) => {
          resolveGet = resolve
        }),
    )
    renderCard()

    expect(await screen.findByRole('status')).toBeVisible()
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Unverified')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    expect(screen.queryByText('加载中')).not.toBeInTheDocument()
    expect(screen.queryByText('Normal')).not.toBeInTheDocument()

    resolveGet!(summary)
    expect(await screen.findByText('Ready')).toBeVisible()
  })

  it.each([
    { readiness: 'ready', label: 'Ready' },
    { readiness: 'not_tested', label: 'Unverified' },
    { readiness: 'unavailable', label: 'Error' },
  ] as const)('locks the $label pill to the fixed header slot', async ({ readiness, label }) => {
    vi.mocked(resource.get).mockResolvedValueOnce({ ...summary, readiness })
    renderCard()

    const pill = await screen.findByText(label)
    expect(pill).toBeVisible()
    // 短文案 Ready/Error 与长文案 Unverified(10) 同槽居中，不推挤开关。
    expect(pill.parentElement).toHaveClass('justify-center')
    expect(pill.parentElement?.style.minWidth).toBe('104px')
    expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe('104px')
    expect(screen.queryByTestId('switch-row-status-placeholder')).not.toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '启用邮件发送' })).toBeVisible()
  })

  it('keeps a same-size placeholder while loading so the switch does not shift', async () => {
    let resolveGet: ((value: MailConfigSummary) => void) | undefined
    vi.mocked(resource.get).mockImplementationOnce(
      () =>
        new Promise<MailConfigSummary>((resolve) => {
          resolveGet = resolve
        }),
    )
    renderCard()

    const slot = await screen.findByTestId('switch-row-status-slot')
    expect(slot.style.minWidth).toBe('104px')
    const placeholder = screen.getByTestId('switch-row-status-placeholder')
    expect(placeholder.style.minWidth).toBe('104px')
    expect(placeholder).toHaveAttribute('aria-hidden', 'true')
    // 占位为空槽：无胶囊文案，既有“加载中无 Status”语义不变。
    expect(placeholder.textContent).toBe('')
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Unverified')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    const toggle = screen.getByRole('switch', { name: '启用邮件发送' })
    expect(toggle).toBeVisible()

    resolveGet!(summary)
    expect(await screen.findByText('Ready')).toBeVisible()
    expect(screen.queryByTestId('switch-row-status-placeholder')).not.toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '启用邮件发送' })).toBeVisible()
  })

  it('keeps the placeholder on load failure without pill text', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('placeholder-probe-load'))
    renderCard()

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('placeholder-probe-load')),
    )
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Unverified')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    const placeholder = screen.getByTestId('switch-row-status-placeholder')
    expect(placeholder.style.minWidth).toBe('104px')
    expect(placeholder).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByRole('switch', { name: '启用邮件发送' })).toBeVisible()
  })
})
