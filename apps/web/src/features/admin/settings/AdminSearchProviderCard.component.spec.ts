import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AdminSearchProviderCard from './AdminSearchProviderCard.vue'
import type { AdminSearchProviderResource } from './searchProviderResource'
import type { SearchProviderConfig } from './searchProviderTypes'

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

function createConfig(overrides: Partial<SearchProviderConfig> = {}): SearchProviderConfig {
  return {
    provider: 'unsplash',
    api_key_configured: true,
    enabled: false,
    test_state: 'never',
    ...overrides,
  }
}

function createResource() {
  return {
    list: vi.fn<AdminSearchProviderResource['list']>(),
    update: vi.fn<AdminSearchProviderResource['update']>().mockResolvedValue(undefined),
    test: vi.fn<AdminSearchProviderResource['test']>(),
  }
}

function renderCard(config: SearchProviderConfig, resource: AdminSearchProviderResource) {
  return renderWithUser(AdminSearchProviderCard, {
    props: { provider: 'unsplash', config, resource, onChange: vi.fn() },
    global: { plugins: [], stubs: { transition: false } },
  })
}

describe('AdminSearchProviderCard', () => {
  let resource: ReturnType<typeof createResource>

  beforeEach(() => {
    vi.clearAllMocks()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReturnValue('test reason')
    resource = createResource()
    resource.test.mockResolvedValue({ provider: 'unsplash', test_state: 'passed', message: 'ok' })
  })

  it('never hydrates the API Key and shows configured badges', async () => {
    renderCard(createConfig(), resource)

    const keyInput = screen.getByLabelText('Unsplash API Key') as HTMLInputElement
    expect(keyInput.value).toBe('')
    expect(keyInput).toHaveAttribute('type', 'password')
    expect(screen.getByText('API Key 已配置')).toBeVisible()
    expect(screen.getByText('未测试')).toBeVisible()
    expect(screen.getAllByText('已停用').length).toBeGreaterThan(0)
  })

  it('disables enable until the current key passes the connection test', async () => {
    renderCard(createConfig({ test_state: 'never' }), resource)

    expect(screen.getByRole('button', { name: '启用' })).toBeDisabled()
    expect(screen.getByText('保存后请先测试连接，通过后才能启用。')).toBeVisible()
  })

  it('blocks saving a blank key through schema validation', async () => {
    const { user } = renderCard(createConfig({ api_key_configured: false }), resource)

    await user.click(screen.getByRole('button', { name: '保存 Key' }))

    expect(await screen.findByText('请输入 API Key 后再保存')).toBeVisible()
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('saves a new key as disabled and requires re-test before enabling', async () => {
    const { user } = renderCard(createConfig({ test_state: 'passed', enabled: true }), resource)

    await user.type(screen.getByLabelText('Unsplash API Key'), 'new-secret')
    await user.click(screen.getByRole('button', { name: '保存 Key' }))

    await waitFor(() =>
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        {
          api_key: 'new-secret',
          enabled: false,
        },
        { operatorReason: 'test reason' },
      ),
    )
    expect(confirmMock).toHaveBeenCalledOnce()
  })

  it('aborts the save when the operator cancels the confirm dialog', async () => {
    confirmMock.mockResolvedValueOnce(false)
    const { user } = renderCard(createConfig({ test_state: 'passed', enabled: true }), resource)

    await user.type(screen.getByLabelText('Unsplash API Key'), 'new-secret')
    await user.click(screen.getByRole('button', { name: '保存 Key' }))

    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledOnce()
    })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('runs the connection test against the saved key', async () => {
    const { user } = renderCard(createConfig({ test_state: 'never' }), resource)

    await user.click(screen.getByRole('button', { name: '测试连接' }))

    await waitFor(() =>
      expect(resource.test).toHaveBeenCalledWith('unsplash', { operatorReason: 'test reason' }),
    )
    expect(toast.success).toHaveBeenCalledWith('Unsplash 连接测试通过')
  })

  it('blocks testing an unsaved key draft', async () => {
    const { user } = renderCard(createConfig({ test_state: 'never' }), resource)

    await user.type(screen.getByLabelText('Unsplash API Key'), 'unsaved-draft')

    expect(screen.getByRole('button', { name: '测试连接' })).toBeDisabled()
    expect(screen.getByText('Key 已修改，先保存再测试。')).toBeVisible()
  })

  it('enables and disables through explicit commands', async () => {
    const { user } = renderCard(createConfig({ test_state: 'passed' }), resource)

    await user.click(screen.getByRole('button', { name: '启用' }))
    await waitFor(() =>
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: 'test reason' },
      ),
    )
  })

  it('surfaces test failure state for retry', async () => {
    vi.mocked(resource.test).mockRejectedValueOnce(new Error('connection refused'))
    const { user } = renderCard(
      createConfig({
        test_state: 'failed',
        failure_category: 'network_timeout',
        status_message: 'connection refused',
      }),
      resource,
    )

    expect(screen.getByText('测试失败')).toBeVisible()
    expect(screen.getByText('connection refused')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '测试连接' }))
    await waitFor(() =>
      expect(resource.test).toHaveBeenCalledWith('unsplash', { operatorReason: 'test reason' }),
    )
  })
})
