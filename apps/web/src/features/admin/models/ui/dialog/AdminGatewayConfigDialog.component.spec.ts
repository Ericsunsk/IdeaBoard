import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, within } from '@testing-library/vue'
import { renderWithUser, screen, waitFor } from '../../../../../test/render'
import AdminGatewayConfigDialog from './AdminGatewayConfigDialog.vue'
import { ADMIN_PROBE_OPERATOR_REASON } from '../../../shared'
import type {
  AdminGatewayConnection,
  GatewayConnectionSummary,
} from '../../resource'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

const mockResource: AdminGatewayConnection = {
  get: vi.fn(),
  update: vi.fn(),
  testConnection: vi.fn(),
}

describe('AdminGatewayConfigDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(mockResource.get).mockResolvedValue({
      base_url: 'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
      account_id: '3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c',
      gateway_id: 'my-gateway',
      token_configured: true,
      is_first_party: true,
    })
    vi.mocked(mockResource.update).mockResolvedValue(undefined)
    vi.mocked(mockResource.testConnection).mockResolvedValue({
      success: true,
      message: '网关连接测试成功',
    })
  })

  function renderDialog(
    props: {
      visible?: boolean
      resource?: AdminGatewayConnection
      onSaved?: (data: GatewayConnectionSummary) => void
      'onUpdate:visible'?: (visible: boolean) => void
    } = {},
  ) {
    return renderWithUser(AdminGatewayConfigDialog, {
      props: {
        visible: props.visible ?? true,
        resource: props.resource ?? mockResource,
        ...(props.onSaved ? { onSaved: props.onSaved } : {}),
        ...(props['onUpdate:visible'] ? { 'onUpdate:visible': props['onUpdate:visible'] } : {}),
      },
      global: {
        stubs: { transition: false },
      },
    })
  }

  it('renders base url and key fields from loaded configuration', async () => {
    renderDialog()

    expect(
      await screen.findByDisplayValue(
        'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
      ),
    ).toBeVisible()
    expect(screen.getByText('已识别 Cloudflare 网关')).toBeVisible()
    expect(screen.getByText(/已配置/)).toBeVisible()
  })

  it('saves custom domain Base URL with Account ID and Gateway ID', async () => {
    vi.mocked(mockResource.get).mockResolvedValue({
      base_url: 'https://my-custom-gateway.company.com',
      token_configured: true,
    })

    const onSaved = vi.fn()
    const onUpdateVisible = vi.fn()
    const { user } = renderDialog({ onSaved, 'onUpdate:visible': onUpdateVisible })

    const baseUrlInput = await screen.findByPlaceholderText(/gateway/i)
    const tokenInput = screen.getByPlaceholderText(/输入网关鉴权 Token/)

    await fireEvent.update(baseUrlInput, 'https://my-custom-gateway.company.com')
    await fireEvent.update(tokenInput, 'cf-secret-token-123')
    await fireEvent.update(screen.getByPlaceholderText(/32 位/), '3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c')
    await fireEvent.update(screen.getByPlaceholderText(/网关标识/), 'my-gateway')

    const saveButton = screen.getByRole('button', { name: '添加网关' })
    await user.click(saveButton)

    expect(mockResource.update).toHaveBeenCalledWith(
      {
        base_url: 'https://my-custom-gateway.company.com',
        token: 'cf-secret-token-123',
        account_id: '3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c',
        gateway_id: 'my-gateway',
      },
      { operatorReason: '' },
    )

    expect(toast.success).toHaveBeenCalledWith('AI 网关配置已保存')
    expect(onSaved).toHaveBeenCalledWith(
      expect.objectContaining({
        base_url: 'https://my-custom-gateway.company.com',
        token_configured: true,
      }),
    )
    expect(onUpdateVisible).toHaveBeenCalledWith(false)
  })

  it('allows custom domain save without Account ID and Gateway ID', async () => {
    // 身份字段选填：不填也能保存，缺身份时由探测报错引导补全。
    vi.mocked(mockResource.get).mockResolvedValue({
      base_url: 'https://my-custom-gateway.company.com',
      token_configured: true,
    })

    const { user } = renderDialog()

    const baseUrlInput = await screen.findByPlaceholderText(/gateway/i)
    await fireEvent.update(baseUrlInput, 'https://my-custom-gateway.company.com')

    const saveButton = screen.getByRole('button', { name: '添加网关' })
    await user.click(saveButton)

    expect(mockResource.update).toHaveBeenCalledWith(
      {
        base_url: 'https://my-custom-gateway.company.com',
        account_id: '',
        gateway_id: '',
      },
      { operatorReason: '' },
    )
    expect(toast.success).toHaveBeenCalledWith('AI 网关配置已保存')
  })

  it('validates invalid URL format', async () => {
    vi.mocked(mockResource.get).mockResolvedValue({
      token_configured: false,
    })

    const { user } = renderDialog()

    const baseUrlInput = await screen.findByPlaceholderText(/gateway/i)
    await fireEvent.update(baseUrlInput, 'not-a-valid-url')

    const saveButton = screen.getByRole('button', { name: '添加网关' })
    await user.click(saveButton)

    expect(screen.getByText('网关 Base URL 必须以 http:// 或 https:// 开头')).toBeVisible()
    expect(mockResource.update).not.toHaveBeenCalled()
  })

  it('keeps existing token when token is left blank during update', async () => {
    const { user } = renderDialog()

    const baseUrlInput = await screen.findByDisplayValue(
      'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
    )
    await fireEvent.update(
      baseUrlInput,
      'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway-v2',
    )

    const saveButton = screen.getByRole('button', { name: '添加网关' })
    await user.click(saveButton)

    expect(mockResource.update).toHaveBeenCalledWith(
      expect.objectContaining({
        base_url:
          'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway-v2',
      }),
      { operatorReason: '' },
    )
    const updateCall = vi.mocked(mockResource.update).mock.calls[0]?.[0]
    expect(updateCall?.token).toBeUndefined()
  })

  it('emits update:visible false when clicking the close (x) button', async () => {
    const onUpdateVisible = vi.fn()
    const { user } = renderDialog({ 'onUpdate:visible': onUpdateVisible })

    const dialog = await screen.findByRole('dialog', { name: '添加网关' })
    const closeBtn = within(dialog).getByRole('button', { name: /close|关闭/i })
    await user.click(closeBtn)

    expect(onUpdateVisible).toHaveBeenCalledWith(false)
  })

  it('emits update:visible false when clicking cancel button', async () => {
    const onUpdateVisible = vi.fn()
    const { user } = renderDialog({ 'onUpdate:visible': onUpdateVisible })

    const cancelBtn = await screen.findByRole('button', { name: '取消' })
    await user.click(cancelBtn)

    expect(onUpdateVisible).toHaveBeenCalledWith(false)
  })

  it('supports testing gateway connection from the modal without a confirm dialog', async () => {
    vi.mocked(mockResource.testConnection).mockResolvedValue({
      success: true,
      latencyMs: 38,
      message: '网关连接正常 (38ms)',
    })

    const { user } = renderDialog()

    await user.click(await screen.findByRole('button', { name: '测试连接' }))
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('测试连接需要填写 provider 与 path')
    })
    expect(mockResource.testConnection).not.toHaveBeenCalled()
    // 失败收敛到 toast：inline 不挂 probe 缺参提示。
    expect(screen.queryByText('测试连接需要填写 provider 与 path')).not.toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('例如: openai'), 'openai')
    await user.type(screen.getByPlaceholderText('例如: v1/models'), 'v1/models')
    const testBtn = await screen.findByRole('button', { name: '测试连接' })
    await user.click(testBtn)

    expect(mockResource.testConnection).toHaveBeenCalledWith(
      expect.objectContaining({
        base_url:
          'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
        provider: 'openai',
        path: 'v1/models',
      }),
      { operatorReason: ADMIN_PROBE_OPERATOR_REASON },
    )
    expect(toast.success).toHaveBeenCalledWith('网关连接正常 (38ms)')
  })

  it('reports unsuccessful probe result only via toast without inline error', async () => {
    vi.mocked(mockResource.testConnection).mockResolvedValue({
      success: false,
      message: '网关探活失败：upstream 502',
    })
    const { user } = renderDialog()

    await screen.findByDisplayValue(
      'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
    )
    await user.type(screen.getByPlaceholderText('例如: openai'), 'openai')
    await user.type(screen.getByPlaceholderText('例如: v1/models'), 'v1/models')
    await user.click(await screen.findByRole('button', { name: '测试连接' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('网关探活失败：upstream 502')
    })
    expect(screen.queryByText('网关探活失败：upstream 502')).not.toBeInTheDocument()
  })

  it('reports probe exception only via toast without inline error', async () => {
    vi.mocked(mockResource.testConnection).mockRejectedValueOnce(new Error('boom'))
    const { user } = renderDialog()

    await screen.findByDisplayValue(
      'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
    )
    await user.type(screen.getByPlaceholderText('例如: openai'), 'openai')
    await user.type(screen.getByPlaceholderText('例如: v1/models'), 'v1/models')
    await user.click(await screen.findByRole('button', { name: '测试连接' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled()
    })
    expect(toast.error.mock.calls[0]?.[0]).toContain('boom')
    expect(screen.queryByText(/boom/)).not.toBeInTheDocument()
  })

  it('keeps base_url validation inline for test connection', async () => {
    const { user } = renderDialog()

    const baseUrlInput = await screen.findByPlaceholderText(/gateway/i)
    await fireEvent.update(baseUrlInput, 'not-a-valid-url')
    await user.type(screen.getByPlaceholderText('例如: openai'), 'openai')
    await user.type(screen.getByPlaceholderText('例如: v1/models'), 'v1/models')
    await user.click(await screen.findByRole('button', { name: '测试连接' }))

    expect(await screen.findByText('网关 Base URL 必须以 http:// 或 https:// 开头')).toBeVisible()
    expect(mockResource.testConnection).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('shows the testing spinner in the probe path input suffix instead of the test button', async () => {
    let resolveTest!: (value: { success: boolean; latencyMs?: number; message?: string }) => void
    vi.mocked(mockResource.testConnection).mockReturnValueOnce(
      new Promise<{ success: boolean; latencyMs?: number; message?: string }>((resolve) => {
        resolveTest = resolve
      }),
    )
    const { user } = renderDialog()

    await screen.findByDisplayValue(
      'https://gateway.ai.cloudflare.com/v1/3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c/my-gateway',
    )
    await user.type(screen.getByPlaceholderText('例如: openai'), 'openai')
    await user.type(screen.getByPlaceholderText('例如: v1/models'), 'v1/models')
    const testBtn = await screen.findByRole('button', { name: '测试连接' })
    await user.click(testBtn)

    // 输入栏尾部出现可访问的 testing 状态，按钮保持 disabled 但自身无 spinner。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('Testing…')
    const spinner = status.querySelector('svg')
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    expect(testBtn).toBeDisabled()
    expect(testBtn.querySelector('svg')).not.toBeInTheDocument()
    // loading 行为：探活输入栏标记 aria-busy（不测内部 Spinner 实现）。
    expect(screen.getByPlaceholderText('例如: v1/models')).toHaveAttribute('aria-busy', 'true')

    resolveTest({ success: true, latencyMs: 38, message: '网关连接正常 (38ms)' })
    await waitFor(() => {
      expect(mockResource.testConnection).toHaveBeenCalledWith(
        expect.objectContaining({ provider: 'openai', path: 'v1/models' }),
        { operatorReason: ADMIN_PROBE_OPERATOR_REASON },
      )
    })
    await waitFor(() => {
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })
    expect(screen.getByPlaceholderText('例如: v1/models')).not.toHaveAttribute('aria-busy')
  })
})
