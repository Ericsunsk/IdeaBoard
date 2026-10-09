import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminImageProvidersTable from './AdminImageProvidersTable.vue'
import { ADMIN_PROBE_OPERATOR_REASON } from '../../shared/lib/confirmSettingsSave'
import type { AdminSearchProviderResource } from './searchProviderResource'
import type { SearchProviderConfig } from './searchProviderTypes'

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

function createConfigs(): SearchProviderConfig[] {
  return [
    {
      provider: 'unsplash',
      api_key_configured: true,
      enabled: false,
      test_state: 'never',
    },
    {
      provider: 'pexels',
      api_key_configured: false,
      enabled: false,
      test_state: 'never',
    },
  ]
}

function createResource() {
  return {
    list: vi.fn<AdminSearchProviderResource['list']>(),
    update: vi.fn<AdminSearchProviderResource['update']>().mockResolvedValue(undefined),
    test: vi.fn<AdminSearchProviderResource['test']>(),
  }
}

describe('AdminImageProvidersTable', () => {
  let resource: ReturnType<typeof createResource>

  beforeEach(() => {
    vi.clearAllMocks()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReturnValue('test reason')
    resource = createResource()
    resource.test.mockResolvedValue({ provider: 'unsplash', test_state: 'passed', message: 'ok' })
  })

  it('renders image library table with providers and status badges', async () => {
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    expect(screen.getByText('图片搜索')).toBeVisible()
    expect(screen.getByText('Unsplash')).toBeVisible()
    expect(screen.getByText('Pexels')).toBeVisible()

    const unsplashKeyInput = screen.getByLabelText('Unsplash API Key') as HTMLInputElement
    expect(unsplashKeyInput.value).toBe('')
    expect(unsplashKeyInput).toHaveAttribute('type', 'password')
    expect(unsplashKeyInput).toHaveAttribute('placeholder', '••••••••••••••••')

    const pexelsKeyInput = screen.getByLabelText('Pexels API Key') as HTMLInputElement
    expect(pexelsKeyInput).toHaveAttribute('placeholder', '请输入 API Key 后再保存')
  })

  it('runs connection test for configured provider', async () => {
    const onChange = vi.fn()
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource, onChange },
    })

    const testButtons = screen.getAllByRole('button', { name: '测试连接' })
    // Unsplash is configured so its test button is enabled
    expect(testButtons[0]).not.toBeDisabled()
    // Pexels has no key configured so disabled
    expect(testButtons[1]).toBeDisabled()

    await user.click(testButtons[0]!)

    await waitFor(() => {
      expect(resource.test).toHaveBeenCalledWith('unsplash', {
        operatorReason: ADMIN_PROBE_OPERATOR_REASON,
      })
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('连接测试通过'))
    })
  })

  it('saves an entered API key with audit confirmation', async () => {
    const onChange = vi.fn()
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource, onChange },
    })

    const pexelsInput = screen.getByLabelText('Pexels API Key')
    await user.type(pexelsInput, 'new-pexels-key-123')

    const saveButtons = screen.getAllByRole('button', { name: '保存' })
    // Pexels save button is enabled
    expect(saveButtons[1]).not.toBeDisabled()

    await user.click(saveButtons[1]!)

    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(resource.update).toHaveBeenCalledWith(
        'pexels',
        { api_key: 'new-pexels-key-123', enabled: false },
        { operatorReason: 'test reason' },
      )
      expect(onChange).toHaveBeenCalledTimes(1)
    })
  })

  it('toggles provider enable state with audit confirmation', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
    ]

    const onChange = vi.fn()
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource, onChange },
    })

    const toggle = screen.getByRole('switch', { name: '启用 Unsplash' })
    expect(toggle).not.toBeDisabled()

    await user.click(toggle)

    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: 'test reason' },
      )
      expect(onChange).toHaveBeenCalledTimes(1)
    })
  })

  it('allows disabling an enabled provider while preserving an unsaved key draft', async () => {
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: {
        configs: [{ ...createConfigs()[0]!, enabled: true, test_state: 'passed' }],
        resource,
      },
    })

    const keyInput = screen.getByLabelText('Unsplash API Key')
    await user.type(keyInput, 'draft-key')

    const toggle = screen.getByRole('switch', { name: '启用 Unsplash' })
    expect(toggle).not.toBeDisabled()
    await user.click(toggle)

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: false },
        { operatorReason: 'test reason' },
      )
    })
    expect(resource.update).toHaveBeenCalledTimes(1)
    expect(keyInput).toHaveValue('draft-key')
  })

  it('blocks enabling a tested provider while a new key draft is unsaved', async () => {
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: {
        configs: [{ ...createConfigs()[0]!, test_state: 'passed' }],
        resource,
      },
    })

    await user.type(screen.getByLabelText('Unsplash API Key'), 'draft-key')

    const toggle = screen.getByRole('switch', { name: '启用 Unsplash' })
    expect(toggle).toBeDisabled()
    await user.click(toggle)
    expect(confirmMock).not.toHaveBeenCalled()
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('shows the api key configured capsule only for configured providers', async () => {
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    expect(screen.getByText('API Key 已配置')).toBeVisible()
    expect(screen.queryByText('API Key 未配置')).not.toBeInTheDocument()
  })

  it('renders table headers from settings messages', async () => {
    const { container } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    const headerText = container.querySelector('thead')?.textContent ?? ''
    expect(headerText).toContain('Status')
    expect(headerText).toContain('操作')
    expect(headerText).not.toContain('连接状态')
  })

  it('renders failure diagnostics from resource data', async () => {
    const failedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'failed',
        failure_category: 'network_timeout',
        status_message: 'connection refused',
        tested_at: '2026-10-01T10:00:00.000Z',
      },
    ]
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: failedConfigs, resource },
    })

    expect(screen.getByText(/失败分类：network_timeout/)).toBeVisible()
    expect(screen.getByText('connection refused')).toBeVisible()
    expect(screen.getByText(/测试时间：/)).toBeVisible()
    expect(screen.getByText('测试失败')).toBeVisible()
  })

  it('renders the last passed test time from resource data', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
        tested_at: '2026-10-01T10:00:00.000Z',
      },
    ]
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    expect(screen.getByText(/上次测试通过：/)).toBeVisible()
    expect(screen.getByText('连接正常，可以启用或停用。')).toBeVisible()
  })

  it('renders no sequence hint when the key is missing and no draft exists', async () => {
    // KeyMissing: 未配置且无草稿，不再提示，由输入框占位与保存按钮引导
    renderWithUser(AdminImageProvidersTable, {
      props: {
        configs: [
          {
            provider: 'pexels',
            api_key_configured: false,
            enabled: false,
            test_state: 'never',
          },
        ],
        resource,
      },
    })
    expect(screen.queryByText('先输入 API Key 并保存。')).not.toBeInTheDocument()
  })

  it('renders the TestFirst sequence hint for untested keys', async () => {
    // TestFirst: 已配置但从未通过测试
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })
    expect(screen.getByText('保存后请先测试连接，通过后才能启用。')).toBeVisible()
  })

  it('switches the sequence hint when the key draft changes', async () => {
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    // KeyChanged: 输入草稿后提示先保存再测试
    await user.type(screen.getByLabelText('Unsplash API Key'), 'draft-key')
    expect(screen.getByText('Key 已修改，先保存再测试。')).toBeVisible()
  })
})
