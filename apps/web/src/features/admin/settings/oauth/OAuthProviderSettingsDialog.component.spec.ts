import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'

vi.mock('../../shared/logos/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))
const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('oauth update'))
vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))
import type { OAuthProviderSettingsResource, OAuthProviderSettingsSummary } from './resource'

const toast = vi.hoisted(() => ({ success: vi.fn() }))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()),
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
    expect(within(dialog).getByText('已启用')).toBeVisible()
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
      expect(resource.save).toHaveBeenCalledWith(
        'google',
        {
          enabled: true,
          clientId: 'google-client.apps.googleusercontent.com',
          clientSecret: '',
        },
        'oauth update',
      ),
    )
    await waitFor(() => expect(resource.get).toHaveBeenCalledTimes(4))
    expect(emitted()['update:visible']).toEqual([[false]])
    expect(toast.success).toHaveBeenCalledWith('Google OAuth 设置已保存')
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
