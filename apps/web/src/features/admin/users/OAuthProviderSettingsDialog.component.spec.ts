import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen, waitFor, within } from '../../../test/render'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'
import type {
  OAuthProviderSettingsResource,
  OAuthProviderSettingsSummary,
} from './oauthProviderSettingsResource'

const toast = vi.hoisted(() => ({ success: vi.fn() }))

vi.mock('../shared/lib/adminToast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../shared/lib/adminToast')>()),
  useAdminToast: () => toast,
}))

const configuredSummary: OAuthProviderSettingsSummary = {
  provider: 'google',
  enabled: true,
  configured: true,
  available: true,
  clientId: 'google-client.apps.googleusercontent.com',
  clientSecretConfigured: true,
  scopes: ['openid', 'email', 'profile'],
  configVersion: 2,
  callbackUrl: 'https://example.com/api/auth/callback/google',
  updatedAt: '2026-08-06T00:00:00Z',
}

const resource: OAuthProviderSettingsResource = {
  get: vi.fn(),
  save: vi.fn(),
  getCapabilities: vi.fn(),
}

function renderDialog() {
  return renderWithUser(OAuthProviderSettingsDialog, {
    props: { visible: true, resource },
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: { transition: false },
    },
  })
}

describe('OAuthProviderSettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(resource.get).mockResolvedValue(configuredSummary)
    vi.mocked(resource.save).mockResolvedValue(configuredSummary)
    vi.mocked(resource.getCapabilities).mockResolvedValue({ social: { google: true } })
  })

  it('shows the generic Provider selector and keeps callback and Secret safe', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')
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
      available: false,
      clientId: '',
      clientSecretConfigured: false,
    })
    const { user } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })

    expect(within(dialog).queryByText('请输入 Client ID')).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: '保存设置' }))

    expect(await within(dialog).findByText('请输入 Client ID')).toBeVisible()
    expect(within(dialog).getByText('请输入 Client Secret')).toBeVisible()
    expect(resource.save).not.toHaveBeenCalled()
  })

  it('allows a blank Secret when one is already configured and refetches both contracts', async () => {
    const { user, emitted } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.click(within(dialog).getByRole('button', { name: '保存设置' }))

    await waitFor(() =>
      expect(resource.save).toHaveBeenCalledWith('google', {
        enabled: true,
        clientId: 'google-client.apps.googleusercontent.com',
        clientSecret: '',
      }),
    )
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(2))
    expect(resource.getCapabilities).toHaveBeenCalledTimes(2)
    expect(emitted()['update:visible']).toEqual([[false]])
    expect(toast.success).toHaveBeenCalledWith('Google OAuth 设置已保存')
  })

  it('renders a retryable load error and disables Save', async () => {
    vi.mocked(resource.get).mockRejectedValueOnce(new Error('auth unavailable'))
    renderDialog()

    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    expect(await within(dialog).findByText('auth unavailable')).toBeVisible()
    expect(within(dialog).getByRole('button', { name: '重新加载' })).toBeEnabled()
    expect(within(dialog).getByRole('button', { name: '保存设置' })).toBeDisabled()
  })

  it('reports a refresh failure as partial success without repeating the saved mutation', async () => {
    vi.mocked(resource.get)
      .mockResolvedValueOnce(configuredSummary)
      .mockRejectedValueOnce(new Error('refresh unavailable'))
    const { user, emitted } = renderDialog()
    const dialog = await screen.findByRole('dialog', { name: 'OAuth Provider 设置' })
    await within(dialog).findByDisplayValue('google-client.apps.googleusercontent.com')

    await user.type(within(dialog).getByLabelText('Client Secret'), 'rotated-secret')
    await user.click(within(dialog).getByRole('button', { name: '保存设置' }))

    expect(await within(dialog).findByText('设置已保存，但状态刷新失败，请重新加载')).toBeVisible()
    expect(resource.save).toHaveBeenCalledTimes(1)
    expect(emitted()['update:visible']).toBeUndefined()
    expect(within(dialog).getByLabelText('Client Secret')).toHaveValue('')
    expect(toast.success).toHaveBeenCalledWith('Google OAuth 设置已保存')
  })
})
