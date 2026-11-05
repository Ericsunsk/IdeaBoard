import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, within } from '@testing-library/vue'
import { renderWithUser, screen } from '@/test/render'
import AdminModelEditDialog from './AdminModelEditDialog.vue'
import type { AdminModelItem } from '../../core/catalogData'

const gatewayFakes = vi.hoisted(() => ({
  previewRoute: vi.fn(),
  listProviders: vi.fn(),
}))

vi.mock('../../resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../resource')>()
  return {
    ...actual,
    createAdminGatewayResource: () => ({
      catalog: {
        previewRoute: gatewayFakes.previewRoute,
        listProviders: gatewayFakes.listProviders,
      },
    }),
  }
})

describe('AdminModelEditDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    gatewayFakes.previewRoute.mockImplementation(
      async (input: { modelId: string; provider?: string }) => {
        const provider = input.provider || ''
        return {
          provider: provider || '未指定',
          model: input.modelId,
          // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
          valid: true,
          // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
          reachable: true,
          status_code: 200,
          latency_ms: 12,
          message: 'Worker 干跑正常 (12ms, HTTP 200)',
        }
      },
    )
    gatewayFakes.listProviders.mockResolvedValue([
      { slug: 'fal', display_name: 'Fal AI', id_prefix: 'fal_ai/' },
      { slug: 'openai', display_name: 'OpenAI', id_prefix: 'openai/' },
      { slug: 'google-vertex-ai', display_name: 'Google Vertex AI', id_prefix: 'vertex_ai/' },
    ])
  })

  const sampleModel: AdminModelItem = {
    id: 'gpt-image-1',
    name: 'GPT-Image-1 创意版',
    provider: 'openai',
    mode: 'image_generation',
    description: '创意画风重绘',
    contextOrResolution: '4096×4096',
    capabilities: ['多尺寸延展', '提示词微调'],
    hosting: '第三方',
    termsUrl: 'https://openai.com/terms',
    docsUrl: 'https://openai.com/docs',
    customPath: 'v1/images/generations',
    customParams: '{"guidance_scale": 7.5}',
  }

  function renderDialog(
    props: {
      visible?: boolean
      model?: AdminModelItem | null
      onSubmit?: (item: AdminModelItem) => void
    } = {},
  ) {
    const { onSubmit, ...rest } = props
    return renderWithUser(AdminModelEditDialog, {
      props: {
        visible: rest.visible ?? true,
        model: rest.model ?? null,
        ...(onSubmit ? { onSubmit } : {}),
      },
      global: {
        stubs: { transition: false },
      },
    })
  }

  it('renders modal in add mode with live preview card and gateway inspector', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '新增模型' })).toBeInTheDocument()

    // 检查左侧核心字段
    expect(within(dialog).getByLabelText(/模型标识/)).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/显示名称/)).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/提供商 \(Provider\)/)).toBeInTheDocument()
    expect(within(dialog).queryByLabelText(/任务类型/)).not.toBeInTheDocument()

    // 检查右侧实时预览
    expect(within(dialog).getByText('卡片效果实时预览')).toBeInTheDocument()
    expect(within(dialog).getByText('Live Preview')).toBeInTheDocument()
    expect(within(dialog).getByText('Edge Worker 干跑透视')).toBeInTheDocument()
  })

  it('updates live preview card and gateway route dynamically as user types', async () => {
    const { user } = renderDialog()

    const dialog = await screen.findByRole('dialog')
    const idInput = within(dialog).getByLabelText(/模型标识/)
    const nameInput = within(dialog).getByLabelText(/显示名称/)
    const descInput = within(dialog).getByLabelText(/功能说明与业务描述/)

    // 模型标识只收裸名，供应商来自下拉真值，预览展示拼接。
    await user.type(idInput, 'birefnet')
    await user.click(await within(dialog).findByRole('button', { name: 'Fal AI' }))
    await user.type(nameInput, 'BiRefNet 高清抠图')
    await user.type(descInput, '发丝级边缘羽化抠图')

    // 实时预览同步
    expect(within(dialog).getByText('BiRefNet 高清抠图')).toBeInTheDocument()
    expect(within(dialog).getByText('发丝级边缘羽化抠图')).toBeInTheDocument()
    expect(within(dialog).getByText('ID: fal/birefnet')).toBeInTheDocument()

    // Worker 干跑经后端推导（防抖 + 异步）：标识有效且干跑可达
    expect(await within(dialog).findByText('Worker 干跑正常 (12ms, HTTP 200)')).toBeInTheDocument()
  })

  it('supports clicking provider preset buttons', async () => {
    const { user } = renderDialog()

    const dialog = await screen.findByRole('dialog')

    // 提供商预设来自 Registry 接口，存 slug 真值（展示 display_name）。
    const falBtn = await within(dialog).findByRole('button', { name: 'Fal AI' })
    expect(within(dialog).getByRole('button', { name: 'Google Vertex AI' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: 'ByteDance' })).not.toBeInTheDocument()
    await user.click(falBtn)

    const providerInput = within(dialog).getByLabelText(/提供商 \(Provider\)/) as HTMLInputElement
    expect(providerInput.value).toBe('fal')
    // 裸名输入不再接受 provider 前缀预填：标识框保持用户手填。
    const idInput = within(dialog).getByLabelText(/模型标识/) as HTMLInputElement
    expect(idInput.value).toBe('')
  })

  it('supports toggling hosting and adding capability presets', async () => {
    const { user } = renderDialog()

    const dialog = await screen.findByRole('dialog')

    // 切换托管类型为第三方
    const thirdPartyBtn = within(dialog).getByRole('button', { name: '第三方托管' })
    await user.click(thirdPartyBtn)

    // 添加能力标签
    const addCapBtn = within(dialog).getByRole('button', { name: '+ 透明通道' })
    await user.click(addCapBtn)

    const capsInput = within(dialog).getByLabelText(/能力标签/) as HTMLInputElement
    expect(capsInput.value).toContain('透明通道')
  })

  it('populates existing data in edit mode and submits updated payload', async () => {
    const submitSpy = vi.fn()
    const { user } = renderDialog({
      model: sampleModel,
      onSubmit: submitSpy,
    })

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '编辑模型' })).toBeInTheDocument()

    // 标识在编辑模式下禁用（存裸名，展示拼接）
    const idInput = within(dialog).getByLabelText(/模型标识/) as HTMLInputElement
    expect(idInput).toBeDisabled()
    expect(idInput.value).toBe('gpt-image-1')
    expect(within(dialog).getByText('ID: openai/gpt-image-1')).toBeInTheDocument()

    // 修改名称
    const nameInput = within(dialog).getByLabelText(/显示名称/)
    await user.clear(nameInput)
    await user.type(nameInput, 'GPT-Image-1 增强版')

    // 提交
    await user.click(within(dialog).getByRole('button', { name: '保存' }))

    expect(submitSpy).toHaveBeenCalledOnce()
    const submitted = submitSpy.mock.calls[0]?.[0] as AdminModelItem
    expect(submitted.name).toBe('GPT-Image-1 增强版')
    expect(submitted.id).toBe('gpt-image-1')
    expect(submitted.provider).toBe('openai')
  })

  it('validates invalid custom JSON parameters before submission', async () => {
    const submitSpy = vi.fn()
    const { user } = renderDialog({
      onSubmit: submitSpy,
    })

    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText(/模型标识/), 'testmodel')
    await user.type(within(dialog).getByLabelText(/显示名称/), 'Test Model')
    await user.type(within(dialog).getByLabelText(/提供商 \(Provider\)/), 'Test')

    // 展开高级设置
    const advancedBtn = within(dialog).getByRole('button', { name: /高级参数微调/ })
    await user.click(advancedBtn)

    // 输入不合法的 JSON
    const jsonInput = within(dialog).getByLabelText(/额外自定义请求参数/)
    await fireEvent.update(jsonInput, '{ invalid_json')

    await user.click(within(dialog).getByRole('button', { name: '确认新增' }))

    expect(screen.getByRole('alert')).toHaveTextContent('额外自定义参数 JSON 格式不合法')
    expect(submitSpy).not.toHaveBeenCalled()
  })
})
