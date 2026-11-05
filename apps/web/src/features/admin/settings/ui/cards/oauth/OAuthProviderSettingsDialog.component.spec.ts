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
  isEnabled: true,
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
    props: { isVisible: true, resource },
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
    expect(within(dialog).queryByText('已启用')).not.toBeInTheDocument()
    expect(within(dialog).queryByText('未启用')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('switch', { name: 'Google 登录' })).toBeChecked()
    expect(within(dialog).getByRole('switch', { name: 'Discord 登录' })).not.toBeChecked()
    expect(within(dialog).getByLabelText('OAuth Provider')).toHaveTextContent('Google')
    const callback = within(dialog).getByLabelText('Callback URL') as HTMLInputElement
    expect(callback).toHaveValue('https://example.com/api/auth/callback/google')
    expect(callback).toHaveAttribute('readonly')
    const secret = within(dialog).getByLabelText('Client Secret') as HTMLInputElement
    expect(secret).toHaveValue('')
    expect(secret).toHaveAttribute('type', 'password')
    expect(secret).toHaveAttribute('placeholder', '已设置，留空则保持不变')
  })

  it('loads the other provider on demand when it is selected', async () => {
    const discordSummary: OAuthProviderSettingsSummary = {
      ...configuredSummary,
      callbackUrl: 'https://example.com/api/auth/callback/discord',
    }
    vi.mocked(resource.get).mockImplementation(async (provider) =>
      provider === 'discord' ? discordSummary : configuredSummary,
    )
    const { user } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')
    expect(vi.mocked(resource.get)).toHaveBeenCalledTimes(1)

    await user.click(within(dialog).getByText('Discord'))

    await within(dialog).findByDisplayValue('https://example.com/api/auth/callback/discord')
    expect(vi.mocked(resource.get)).toHaveBeenCalledTimes(2)
    expect(vi.mocked(resource.get)).toHaveBeenNthCalledWith(2, 'discord')
    expect(within(dialog).getByRole('switch', { name: 'Discord 登录' })).toBeChecked()
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

  it('prevents saving another provider credentials after a provider load fails', async () => {
    const { user, rerender } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('Discord unavailable'))

    await user.click(within(dialog).getByText('Discord'))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Discord unavailable'))
    const saveButton = within(dialog).getByRole('button', { name: '保存' })
    expect(saveButton).toBeDisabled()
    expect(within(dialog).queryByDisplayValue(configuredSummary.clientId)).not.toBeInTheDocument()
    await user.click(saveButton)
    expect(resource.save).not.toHaveBeenCalled()

    vi.mocked(resource.get).mockResolvedValueOnce({
      ...configuredSummary,
      clientId: 'discord-client',
      callbackUrl: 'https://example.com/api/auth/callback/discord',
    })
    await rerender({ isVisible: false })
    await rerender({ isVisible: true })
    await within(dialog).findByDisplayValue('discord-client')
    await user.click(within(dialog).getByRole('button', { name: '保存' }))
    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith('discord', {
        isEnabled: true,
        clientId: 'discord-client',
        clientSecret: '',
      }),
    )
  })

  it('allows a blank Secret when one is already configured and closes without a refresh roundtrip', async () => {
    const { user, emitted } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith('google', {
        isEnabled: true,
        clientId: 'google-client.apps.googleusercontent.com',
        clientSecret: '',
      }),
    )
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(1))
    expect(emitted()['update:isVisible']).toEqual([[false]])
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
    expect(within(dialog).queryByText(/toast-probe-save/)).not.toBeInTheDocument()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
    expect(within(dialog).queryByText('已启用')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('switch', { name: 'Google 登录' })).toBeChecked()
  })

  it('routes incomplete provider to toast error without an inline alert', async () => {
    vi.mocked(resource.get).mockImplementation(async (provider) =>
      provider === 'discord'
        ? {
            ...configuredSummary,
            isEnabled: false,
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
        expect.stringContaining('请先设置 Discord 的 Client ID 和 Client Secret'),
      ),
    )
    expect(resource.save).not.toHaveBeenCalled()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
  })
  it('routes a load failure to toast without an inline error', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('auth unavailable'))
    renderDialog()

    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('auth unavailable'))
    expect(within(dialog).queryByText('auth unavailable')).not.toBeInTheDocument()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: '重新加载' })).not.toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '保存' })).toBeDisabled()
  })
})
