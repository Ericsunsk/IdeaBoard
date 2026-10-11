import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../../../test/render'
import AdminImageProvidersTable from './AdminImageProvidersTable.vue'
import type { AdminSearchProviderResource } from '../../../session/image-library/searchProviderResource'
import type { SearchProviderConfig, SearchProviderTestResult } from '../../../session/image-library/searchProviderTypes'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('../../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
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
        operatorReason: '',
      })
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('连接测试通过'))
    })
  })

  it('shows the testing spinner in the api key input suffix instead of the test button', async () => {
    let resolveTest!: (value: SearchProviderTestResult) => void
    resource.test.mockReturnValueOnce(
      new Promise<SearchProviderTestResult>((resolve) => {
        resolveTest = resolve
      }),
    )
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    const testButtons = screen.getAllByRole('button', { name: '测试连接' })
    await user.click(testButtons[0]!)

    // 输入栏尾部出现可访问的 testing 状态，按钮保持 disabled 但自身无 spinner。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('Testing…')
    const spinner = status.querySelector('svg')
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    expect(testButtons[0]).toBeDisabled()
    expect(testButtons[0]!.querySelector('svg')).not.toBeInTheDocument()
    // loading 行为：输入栏标记 aria-busy（不测内部 Spinner 实现）。
    expect(screen.getByLabelText('Unsplash API Key')).toHaveAttribute('aria-busy', 'true')

    resolveTest({ provider: 'unsplash', test_state: 'passed', message: 'ok' })
    await waitFor(() => {
      expect(resource.test).toHaveBeenCalledWith('unsplash', { operatorReason: '' })
    })
    await waitFor(() => {
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })
    expect(screen.getByLabelText('Unsplash API Key')).not.toHaveAttribute('aria-busy')
  })

  it('saves an entered API key without a confirm dialog', async () => {
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
      expect(resource.update).toHaveBeenCalledWith(
        'pexels',
        { api_key: 'new-pexels-key-123', enabled: false },
        { operatorReason: '' },
      )
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('API Key 已保存'))
    })
  })

  it('toggles provider enable state without a confirm dialog', async () => {
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

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(toggle).not.toBeDisabled()

    await user.click(toggle)

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
      expect(onChange).toHaveBeenCalledTimes(1)
      expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('已启用'))
    })
  })

  it('blocks an API key save while the same provider toggle is pending', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
      { ...createConfigs()[1]!, api_key_configured: true, test_state: 'passed' },
    ]
    let resolveToggle!: (value: void) => void
    resource.update.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveToggle = resolve
      }),
    )
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    await user.click(screen.getByRole('switch', { name: 'Enable Unsplash' }))
    await user.type(screen.getByLabelText('Unsplash API Key'), 'draft-key')

    const save = screen.getAllByRole('button', { name: '保存' })[0]!
    expect(save).toBeDisabled()
    await user.click(save)
    expect(resource.update).toHaveBeenCalledTimes(1)
    const tests = screen.getAllByRole('button', { name: '测试连接' })
    expect(tests[0]).toBeDisabled()
    expect(tests[1]).not.toBeDisabled()
    await user.click(tests[1]!)
    expect(resource.test).toHaveBeenCalledWith('pexels', { operatorReason: '' })

    resolveToggle()
    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
    })
  })

  it('blocks toggles and connection tests while an API key save is pending', async () => {
    let resolveSave!: () => void
    resource.update.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveSave = resolve
      }),
    )
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: {
        configs: [{ ...createConfigs()[0]!, enabled: true, test_state: 'passed' }],
        resource,
      },
    })
    await user.type(screen.getByLabelText('Unsplash API Key'), 'replacement-key')
    await user.click(screen.getByRole('button', { name: '保存' }))

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    const test = screen.getByRole('button', { name: '测试连接' })
    expect(toggle).toBeDisabled()
    expect(test).toBeDisabled()
    await user.click(toggle)
    await user.click(test)
    expect(resource.update).toHaveBeenCalledTimes(1)
    expect(resource.test).not.toHaveBeenCalled()

    resolveSave()
    await waitFor(() => expect(screen.getByLabelText('Unsplash API Key')).not.toBeDisabled())
    expect(toggle).toHaveAttribute('data-state', 'unchecked')
    expect(toggle).toBeDisabled()
  })

  it('keeps the enabled projection and draft when key save fails before backfill', async () => {
    resource.update.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('failed-key'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: [{ ...createConfigs()[0]!, test_state: 'passed' }], resource },
    })
    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    await user.click(toggle)
    await waitFor(() => expect(toggle).not.toBeDisabled())
    const input = screen.getByLabelText('Unsplash API Key')
    await user.type(input, 'replacement-key')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('failed-key'))
      expect(toggle).toHaveAttribute('data-state', 'checked')
      expect(input).toHaveValue('replacement-key')
    })
  })

  it('commits disabled after saving a key when the toggle result is not backfilled yet', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
    ]
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    await user.click(screen.getByRole('switch', { name: 'Enable Unsplash' }))
    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
    })
    await user.type(screen.getByLabelText('Unsplash API Key'), 'replacement-key')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { api_key: 'replacement-key', enabled: false },
        { operatorReason: '' },
      )
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
        'data-state',
        'unchecked',
      )
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toBeDisabled()
      expect(screen.getByText('Unverified')).toBeVisible()
    })

    await user.click(screen.getByRole('button', { name: '测试连接' }))
    await waitFor(() => {
      expect(resource.test).toHaveBeenCalledWith('unsplash', { operatorReason: '' })
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).not.toBeDisabled()
    })
  })

  it('preserves the committed enable state when a later toggle fails before backfill', async () => {
    resource.update
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('failed-disable'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: [{ ...createConfigs()[0]!, test_state: 'passed' }], resource },
    })
    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })

    await user.click(toggle)
    await waitFor(() => expect(toggle).not.toBeDisabled())
    await user.click(toggle)

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('failed-disable'))
      expect(toggle).toHaveAttribute('data-state', 'checked')
    })
    expect(resource.update).toHaveBeenCalledTimes(2)
  })

  it('blocks enable after saving a key if the connection test fails before backfill', async () => {
    resource.test.mockRejectedValueOnce(new Error('replacement-test-failed'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: {
        configs: [{ ...createConfigs()[0]!, enabled: true, test_state: 'passed' }],
        resource,
      },
    })
    await user.type(screen.getByLabelText('Unsplash API Key'), 'replacement-key')
    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '测试连接' })).not.toBeDisabled())
    await user.click(screen.getByRole('button', { name: '测试连接' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('replacement-test-failed'))
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toBeDisabled()
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
        'data-state',
        'unchecked',
      )
    })
  })

  it('allows testing a newly saved key and releases local state after server backfill', async () => {
    const view = renderWithUser(AdminImageProvidersTable, {
      props: { configs: [createConfigs()[1]!], resource },
    })
    resource.test.mockResolvedValueOnce({ provider: 'pexels', test_state: 'passed', message: 'ok' })
    await view.user.type(screen.getByLabelText('Pexels API Key'), 'new-key')
    await view.user.click(screen.getByRole('button', { name: '保存' }))
    const test = screen.getByRole('button', { name: '测试连接' })
    await waitFor(() => expect(test).not.toBeDisabled())
    await view.user.click(test)
    const toggle = screen.getByRole('switch', { name: 'Enable Pexels' })
    await waitFor(() => {
      expect(toggle).not.toBeDisabled()
      expect(screen.getByText('Ready')).toBeVisible()
    })

    await view.rerender({
      configs: [{ ...createConfigs()[1]!, api_key_configured: true, test_state: 'passed' }],
      resource,
    })
    await view.rerender({
      configs: [
        { ...createConfigs()[1]!, api_key_configured: true, enabled: true, test_state: 'failed' },
      ],
      resource,
    })
    expect(toggle).toHaveAttribute('data-state', 'checked')
    expect(screen.getByText('Error')).toBeVisible()
    expect(toggle).not.toBeDisabled()
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

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(toggle).not.toBeDisabled()
    await user.click(toggle)

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: false },
        { operatorReason: '' },
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

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(toggle).toBeDisabled()
    await user.click(toggle)
    expect(resource.update).not.toHaveBeenCalled()
  })

  it('does not render the api key configured capsule (toast 精简后)', async () => {
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    // 精简目标：删除 Service 列的 `API Key 已配置`胶囊，Status 列为唯一真相源。
    expect(screen.queryByText('API Key 已配置')).not.toBeInTheDocument()
    expect(screen.queryByText('API Key 未配置')).not.toBeInTheDocument()
  })

  it('renders table headers from settings messages', async () => {
    const { container } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    const headerText = container.querySelector('thead')?.textContent ?? ''
    expect(headerText).toContain('Status')
    expect(headerText).toContain('操作')
    expect(headerText).not.toContain('状态')
    expect(headerText).not.toContain('连接状态')
  })

  it('renders Status pills as the source of truth for all test states', async () => {
    const mixedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
      {
        provider: 'pexels',
        api_key_configured: false,
        enabled: false,
        test_state: 'never',
      },
    ]
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: mixedConfigs, resource },
    })

    // 健康三态（方案 A）：passed→Ready(success)、never→Unverified(neutral)。
    const ready = screen.getByText('Ready')
    expect(ready).toBeVisible()
    expect(ready.parentElement?.className ?? '').toContain('bg-success-soft')
    const unverified = screen.getByText('Unverified')
    expect(unverified).toBeVisible()
    expect(unverified.parentElement?.className ?? '').toContain('bg-muted')
    expect(screen.queryByText('连接正常')).not.toBeInTheDocument()
    expect(screen.queryByText('未测试')).not.toBeInTheDocument()
  })

  it('renders the failed Status pill without inline failure diagnostics', async () => {
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

    // Status 列保留为真相源（方案 A：failed→Error danger，统一 AdminStatusPill）。
    const abnormal = screen.getByText('Error')
    expect(abnormal).toBeVisible()
    expect(abnormal.parentElement?.className ?? '').toContain('bg-destructive-soft')
    expect(screen.queryByText('测试失败')).not.toBeInTheDocument()
    // 精简目标：内嵌失败诊断块已删除，失败详情一律走 toast。
    expect(screen.queryByText(/失败分类：/)).not.toBeInTheDocument()
    expect(screen.queryByText('connection refused')).not.toBeInTheDocument()
    expect(screen.queryByText(/测试时间：/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('does not render the last-passed hint text', async () => {
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

    // Status 列仍展示 Ready，但行内“上次测试通过/可以启用”提示已删除。
    expect(screen.getByText('Ready')).toBeVisible()
    expect(screen.queryByText('连接正常')).not.toBeInTheDocument()
    expect(screen.queryByText(/上次测试通过：/)).not.toBeInTheDocument()
    expect(screen.queryByText('连接正常，可以启用或停用。')).not.toBeInTheDocument()
  })

  it('does not render sequence hints for untested keys', async () => {
    // TestFirst 提示已删除：已配置但从未通过测试也不再提示行内文案。
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })
    expect(screen.queryByText('保存后请先测试连接，通过后才能启用。')).not.toBeInTheDocument()
    expect(screen.queryByText('先输入 API Key 并保存。')).not.toBeInTheDocument()
  })

  it('does not render a sequence hint when the key draft changes', async () => {
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    // KeyChanged 提示已删除：输入草稿后也不再提示行内文案。
    await user.type(screen.getByLabelText('Unsplash API Key'), 'draft-key')
    expect(screen.queryByText('Key 已修改，先保存再测试。')).not.toBeInTheDocument()
    expect(screen.queryByText('保存后请先测试连接，通过后才能启用。')).not.toBeInTheDocument()
    expect(screen.queryByText('连接正常，可以启用或停用。')).not.toBeInTheDocument()
  })

  it('routes connection test failure to toast error without a global alert', async () => {
    resource.test.mockRejectedValueOnce(new Error('toast-probe-test'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    const testButtons = screen.getAllByRole('button', { name: '测试连接' })
    await user.click(testButtons[0]!)

    await waitFor(() => {
      expect(resource.test).toHaveBeenCalledWith('unsplash', { operatorReason: '' })
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-test'))
    })
    expect(toast.success).not.toHaveBeenCalled()
    // 精简目标：底部 globalError AdminAlert 已删除，失败一律 toast。
    expect(screen.queryByText(/toast-probe-test/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('routes save failure to toast error without a global alert', async () => {
    resource.update.mockRejectedValueOnce(new Error('toast-probe-save'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    await user.type(screen.getByLabelText('Pexels API Key'), 'new-pexels-key-123')
    const saveButtons = screen.getAllByRole('button', { name: '保存' })
    await user.click(saveButtons[1]!)

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'pexels',
        { api_key: 'new-pexels-key-123', enabled: false },
        { operatorReason: '' },
      )
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-save'))
    })
    expect(toast.success).not.toHaveBeenCalled()
    expect(screen.queryByText(/toast-probe-save/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('routes toggle failure to toast error without a global alert', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
    ]
    resource.update.mockRejectedValueOnce(new Error('toast-probe-toggle'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    await user.click(screen.getByRole('switch', { name: 'Enable Unsplash' }))

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-toggle'))
    })
    expect(toast.success).not.toHaveBeenCalled()
    expect(screen.queryByText(/toast-probe-toggle/)).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('flips the switch optimistically without remounting the row (toggle jitter regression)', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
    ]
    let resolveUpdate!: (value: void) => void
    resource.update.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveUpdate = resolve
      }),
    )
    const view = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    const rowBefore = toggle.closest('tr')
    expect(rowBefore).not.toBeNull()

    await view.user.click(toggle)

    // 乐观翻转：update 尚未 resolve，开关已翻到 checked，不等父级 refetch。
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
        'data-state',
        'checked',
      )
    })
    // 开关 x 不漂：toggle 走独立 pending，同行保存按钮不挂 loading spinner。
    const saveButtons = screen.getAllByRole('button', { name: '保存' })
    expect(saveButtons[0]?.querySelector('svg')).not.toBeInTheDocument()
    expect(saveButtons[0]).not.toHaveAttribute('aria-busy')

    resolveUpdate()
    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
    })

    // 模拟父级 emit('change') 后 refetch 整体替换 configs：同行 DOM 复用。
    await view.rerender({
      configs: [{ ...passedConfigs[0]!, enabled: true }],
      resource,
    })
    const toggleAfter = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(toggleAfter).toHaveAttribute('data-state', 'checked')
    expect(toggleAfter.closest('tr')).toBe(rowBefore)
    expect(toggleAfter.closest('tr')).toBeInTheDocument()
  })

  it('rolls the optimistic switch back on toggle failure without remounting the row', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
    ]
    resource.update.mockRejectedValueOnce(new Error('toggle-rollback'))
    const { user } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    const toggle = screen.getByRole('switch', { name: 'Enable Unsplash' })
    const rowBefore = toggle.closest('tr')

    await user.click(toggle)

    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
    })
    // 失败回滚：开关回到 unchecked，同行 DOM 不换。
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
        'data-state',
        'unchecked',
      )
    })
    expect(screen.getByRole('switch', { name: 'Enable Unsplash' }).closest('tr')).toBe(rowBefore)
  })

  it('reserves input right padding so the testing suffix does not shift layout', async () => {
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    // 非 loading 也预留 pr-9：testing spinner 挂载时输入宽度不变。
    const input = screen.getByLabelText('Unsplash API Key')
    expect(input.className).toContain('pr-9')
  })

  it('keeps row and key cells stable without disabled/opacity snaps through toggle + backfill', async () => {
    const passedConfigs: SearchProviderConfig[] = [
      {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      },
      {
        provider: 'pexels',
        api_key_configured: false,
        enabled: false,
        test_state: 'never',
      },
    ]
    let resolveUpdate!: (value: void) => void
    resource.update.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveUpdate = resolve
      }),
    )
    const view = renderWithUser(AdminImageProvidersTable, {
      props: { configs: passedConfigs, resource },
    })

    const toggleBefore = screen.getByRole('switch', { name: 'Enable Unsplash' })
    const rowBefore = toggleBefore.closest('tr')
    const pillBefore = screen.getByText('Ready')
    const pexelsPillBefore = screen.getByText('Unverified')
    const inputBefore = screen.getByLabelText('Unsplash API Key')
    expect(rowBefore).not.toBeNull()
    expect(rowBefore!.className).not.toContain('opacity-')

    await view.user.click(toggleBefore)

    // 乐观翻转已生效，pending 只压开关：开关 disabled（自带 opacity 过渡），
    // 同行输入 disabled 不翻，行不挂 opacity 弱示意类。
    await waitFor(() => {
      expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
        'data-state',
        'checked',
      )
    })
    const togglePending = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(togglePending).toBeDisabled()
    expect(screen.getByLabelText('Unsplash API Key')).not.toBeDisabled()
    expect(togglePending.closest('tr')).toBe(rowBefore)
    expect(togglePending.closest('tr')!.className).not.toContain('opacity-')

    resolveUpdate()
    await waitFor(() => {
      expect(resource.update).toHaveBeenCalledWith(
        'unsplash',
        { enabled: true },
        { operatorReason: '' },
      )
    })

    // 父级回填：全新对象 + 后台 loading，不换行 DOM/关键 cell 引用，不挂 opacity。
    await view.rerender({
      configs: [
        {
          provider: 'unsplash',
          api_key_configured: true,
          enabled: true,
          test_state: 'passed',
        },
        {
          provider: 'pexels',
          api_key_configured: false,
          enabled: false,
          test_state: 'never',
        },
      ],
      resource,
      loading: true,
    })
    const toggleAfter = screen.getByRole('switch', { name: 'Enable Unsplash' })
    expect(toggleAfter).toHaveAttribute('data-state', 'checked')
    expect(toggleAfter.closest('tr')).toBe(rowBefore)
    expect(toggleAfter).toBe(togglePending)
    expect(screen.getByText('Ready')).toBe(pillBefore)
    expect(screen.getByText('Unverified')).toBe(pexelsPillBefore)
    expect(screen.getByLabelText('Unsplash API Key')).toBe(inputBefore)
    expect(toggleAfter.closest('tr')!.className).not.toContain('opacity-')
    // 后台回填不透 loading：table 不挂 aria-busy，行保留挂载。
    expect(view.container.querySelector('table')).not.toHaveAttribute('aria-busy')
  })

  it('transitions status pill color instead of hard-cutting on test_state change', async () => {
    renderWithUser(AdminImageProvidersTable, {
      props: { configs: createConfigs(), resource },
    })

    // Badge 本体自带 transition-colors，包一层把过渡带给圆点 span。
    const badges = screen.getAllByText('Unverified')
    expect(badges.length).toBeGreaterThan(0)
    for (const pill of badges) {
      const badge = pill.closest('div')
      expect(badge?.parentElement?.className).toContain('[&_span]:transition-colors')
    }
  })

  it('still surfaces loading state for the initial empty fetch', async () => {
    const { container } = renderWithUser(AdminImageProvidersTable, {
      props: { configs: [], resource, loading: true },
    })

    // 空表初载：loading 透给 AdminTable，骨架/aria-busy 不变。
    expect(container.querySelector('table')).toHaveAttribute('aria-busy', 'true')
  })
})
