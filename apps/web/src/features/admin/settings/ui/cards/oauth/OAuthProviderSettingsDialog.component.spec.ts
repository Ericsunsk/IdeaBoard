import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '@/test/render'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'

vi.mock('../../../../shared/session/brand/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))
import type {
  OAuthProviderSettingsResource,
  OAuthProviderSettingsSummary,
} from '../../../session/oauth/resource'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('../../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

const configuredSummary: OAuthProviderSettingsSummary = {
  enabled: true,
  configured: true,
  clientId: 'google-client.apps.googleusercontent.com',
  clientSecretConfigured: true,
  callbackUrl: 'https://example.com/api/auth/callback/google',
}

const resource: OAuthProviderSettingsResource = {
  get: vi.fn(),
  save: vi.fn(),
}

function renderDialog() {
  return renderWithUser(OAuthProviderSettingsDialog, {
    props: { visible: true, resource },
    global: {
      plugins: [],
      stubs: { transition: false },
    },
  })
}

describe('OAuthProviderSettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(resource.get).mockResolvedValue(configuredSummary)
    vi.mocked(resource.save).mockResolvedValue(configuredSummary)
  })

  it('shows the generic Provider selector and keeps callback and Secret safe', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')
    expect(within(dialog).getByRole('group', { name: 'OAuth 登录方式' })).toBeVisible()
    expect(within(dialog).getAllByRole('switch')).toHaveLength(2)
    expect(within(dialog).queryByText('Google 登录')).not.toBeInTheDocument()
    // 方案 A：启用表达只用开关，凭据区冗余 Status pill 已删除。
    expect(within(dialog).queryByText('已启用')).not.toBeInTheDocument()
    expect(within(dialog).queryByText('未启用')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('switch', { name: 'Google 登录' })).toBeChecked()
    expect(within(dialog).getByRole('switch', { name: 'Discord 登录' })).toBeChecked()
    expect(within(dialog).getByLabelText('OAuth Provider')).toHaveTextContent('Google')
    const callback = within(dialog).getByLabelText('Callback URL') as HTMLInputElement
    expect(callback).toHaveValue('https://example.com/api/auth/callback/google')
    expect(callback).toHaveAttribute('readonly')
    const secret = within(dialog).getByLabelText('Client Secret') as HTMLInputElement
    expect(secret).toHaveValue('')
    expect(secret).toHaveAttribute('type', 'password')
    expect(secret).toHaveAttribute('placeholder', '已配置，留空则保持不变')
  })

  it('validates incomplete enabled settings only when Save is clicked', async () => {
    vi.mocked(resource.get).mockResolvedValueOnce({
      ...configuredSummary,
      configured: false,
      clientId: '',
      clientSecretConfigured: false,
    })
    const { user } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })

    expect(within(dialog).queryByText('请输入 Client ID')).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    expect(await within(dialog).findByText('请输入 Client ID')).toBeVisible()
    expect(within(dialog).getByText('请输入 Client Secret')).toBeVisible()
    expect(resource.save).not.toHaveBeenCalled()
  })

  it('allows a blank Secret when one is already configured and refetches both providers', async () => {
    const { user, emitted } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith('google', {
        enabled: true,
        clientId: 'google-client.apps.googleusercontent.com',
        clientSecret: '',
      }),
    )
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(4))
    expect(emitted()['update:visible']).toEqual([[false]])
    expect(toast.success).toHaveBeenCalledWith('Google OAuth 设置已保存')
  })

  it('routes save failure to toast error without an inline alert', async () => {
    vi.mocked(resource.save).mockRejectedValueOnce(new Error('toast-probe-save'))
    const { user } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-save')),
    )
    expect(toast.success).not.toHaveBeenCalled()
    // 精简目标：saveError 内嵌 AdminAlert 已删除，失败一律 toast。
    expect(within(dialog).queryByText(/toast-probe-save/)).not.toBeInTheDocument()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
    // 方案 A：启用真相源是开关（Status pill 已删除），弹窗保持打开可重试。
    expect(within(dialog).queryByText('已启用')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('switch', { name: 'Google 登录' })).toBeChecked()
  })

  it('routes incomplete provider to toast error without an inline alert', async () => {
    vi.mocked(resource.get).mockImplementation(async (provider) =>
      provider === 'discord'
        ? {
            ...configuredSummary,
            enabled: false,
            configured: false,
            clientId: '',
            clientSecretConfigured: false,
          }
        : configuredSummary,
    )
    const { user } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    const switches = within(dialog).getAllByRole('switch')
    await user.click(switches[1]!)
    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringContaining('请先配置 Discord 的 Client ID 和 Client Secret'),
      ),
    )
    expect(resource.save).not.toHaveBeenCalled()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
  })
  it('renders a retryable load error and disables Save', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('auth unavailable'))
    renderDialog()

    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    expect(await within(dialog).findByText('auth unavailable')).toBeVisible()
    expect(within(dialog).getByRole('button', { name: '重新加载' })).toBeEnabled()
    expect(within(dialog).getByRole('button', { name: '保存' })).toBeDisabled()
  })

  it('reports a refresh failure as partial success without repeating the saved mutation', async () => {
    vi.mocked(resource.get)
      .mockResolvedValueOnce(configuredSummary)
      .mockResolvedValueOnce(configuredSummary)
      .mockRejectedValueOnce(new Error('refresh unavailable'))
    const { user, emitted } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.type(within(dialog).getByLabelText('Client Secret'), 'rotated-secret')
    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    expect(await within(dialog).findByText('设置已保存，但状态刷新失败，请重新加载')).toBeVisible()
    expect(resource.save).toHaveBeenCalledTimes(1)
    expect(emitted()['update:visible']).toBeUndefined()
    expect(within(dialog).getByLabelText('Client Secret')).toHaveValue('')
    expect(toast.success).toHaveBeenCalledWith('Google OAuth 设置已保存')
  })
})
