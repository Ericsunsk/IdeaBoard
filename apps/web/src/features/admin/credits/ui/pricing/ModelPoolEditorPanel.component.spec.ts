import { defineComponent, h, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import ModelPoolEditorPanel from './ModelPoolEditorPanel.vue'
import type {
  AdminBillableItemCard,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
} from '../../core/pricing/billableCatalogTypes'

const item: AdminBillableItemCard = {
  sku: 'ai-image:generation',
  namespace: 'ai-image',
  productKind: 'workflow-feature',
  displayName: '图片生成',
  sortOrder: 1,
  status: 'active',
  pricingVariants: ['1k', '2k', '4k'],
  currentPrices: {},
}

const entries: AdminModelPoolEntry[] = [
  { modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
  { modelId: 'b/second', enabled: false, resolutions: [] },
]

const catalogModels: AdminPoolCatalogModel[] = [
  { id: 'a/first', name: 'First' },
  { id: 'b/second', name: 'Second' },
  { id: 'c/third', name: 'Third' },
]

const TestHarness = defineComponent({
  name: 'ModelPoolEditorPanelHarness',
  props: {
    item: { type: Object as () => AdminBillableItemCard, default: () => item },
    entries: { type: Array as () => AdminModelPoolEntry[], default: () => entries },
    catalogModels: { type: Array as () => AdminPoolCatalogModel[], default: () => catalogModels },
    catalogLoading: { type: Boolean, default: false },
    pending: { type: Boolean, default: false },
    errorMessage: { type: String, default: '' },
    stale: { type: Boolean, default: false },
    saveDisabled: { type: Boolean, default: false },
    catalogError: { type: String, default: '' },
  },
  emits: ['save', 'reload', 'reloadCatalog'],
  setup(props, { emit }) {
    const panelRef = ref<InstanceType<typeof ModelPoolEditorPanel> | null>(null)
    return () =>
      h('div', [
        h(ModelPoolEditorPanel, {
          ref: panelRef,
          ...props,
          onReload: () => emit('reload'),
          onReloadCatalog: () => emit('reloadCatalog'),
        }),
        h(
          'button',
          {
            type: 'button',
            onClick: () => {
              if (props.saveDisabled || !panelRef.value?.checkPoolValidity()) return
              emit('save', panelRef.value.getDraftEntries())
            },
          },
          '保存',
        ),
      ])
  },
})

function renderPanel(overrides: Record<string, unknown> = {}) {
  return renderWithUser(TestHarness, {
    props: {
      item,
      entries,
      catalogModels,
      ...overrides,
    },
    // reka `Select` 把选项渲染进 portal，jsdom 里打不开（见
    // TaskControlToolbar.browser.spec.ts 的说明）。草稿增删逻辑与选择器
    // 键盘模型无关，此处把 AdminSelect 桩成原生 select 驱动流程；
    // 选择器自身的语义由下面的 combobox 用例用真实组件断言。
    global: {
      stubs: {
        AdminSelect: defineComponent({
          name: 'AdminSelect',
          props: {
            modelValue: { type: [String, Number], default: '' },
            options: {
              type: Array as () => Array<{ label: string; value: string | number }>,
              default: () => [],
            },
            placeholder: { type: String, default: '' },
            disabled: { type: Boolean, default: false },
          },
          emits: ['update:modelValue'],
          setup(props, { emit, attrs }) {
            return () =>
              h(
                'select',
                {
                  value: props.modelValue,
                  'aria-label': (attrs['aria-label'] as string | undefined) ?? props.placeholder,
                  disabled: props.disabled,
                  onChange: (event: Event) =>
                    emit('update:modelValue', (event.target as HTMLSelectElement).value),
                },
                [
                  h('option', { value: '', disabled: true }, props.placeholder),
                  ...(props.options ?? []).map((opt) =>
                    h('option', { key: String(opt.value), value: String(opt.value) }, opt.label),
                  ),
                ],
              )
          },
        }),
      },
    },
  })
}

describe('ModelPoolEditorPanel', () => {
  it('renders entries with resolution badges', async () => {
    renderPanel()

    expect(await screen.findByText('a/first')).toBeVisible()
    expect(screen.getByText('b/second')).toBeVisible()
    expect(screen.queryByText(/版本/)).not.toBeInTheDocument()
    // 1k badge appears once (a/first); 2k once; 4k toggle buttons exist per entry.
    expect(screen.getByRole('button', { name: 'a/first1K分辨率已支持' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'b/second1K分辨率未支持' })).toBeVisible()
    expect(screen.queryByText(/priority|fallback|weight/i)).not.toBeInTheDocument()
  })

  it('toggles a resolution badge and exposes the updated draft for dialog save', async () => {
    const { user, emitted } = renderPanel()

    await user.click(await screen.findByRole('button', { name: 'b/second4K分辨率未支持' }))
    expect(await screen.findByRole('button', { name: 'b/second4K分辨率已支持' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(emitted().save).toHaveLength(1)
    expect(emitted().save![0]).toEqual([
      [
        { modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
        { modelId: 'b/second', enabled: false, resolutions: ['4k'] },
      ],
    ])
  })

  it('blocks save when an enabled model has no resolution', async () => {
    const { user, emitted } = renderPanel({
      entries: [{ modelId: 'a/first', enabled: true, resolutions: [] }],
    })

    await user.click(await screen.findByRole('button', { name: '保存' }))
    expect(await screen.findByText('启用的模型至少需要一个分辨率')).toBeVisible()
    expect(emitted().save).toBeUndefined()
  })

  it('mounts the catalog picker on AdminSelect with the accessible label', async () => {
    renderWithUser(TestHarness, {
      props: {
        item,
        entries,
        catalogModels,
      },
    })

    const trigger = await screen.findByRole('combobox', { name: '选择模型' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).toHaveTextContent('选择模型')
  })

  it('adds a catalog model and removes an entry', async () => {
    const { user, emitted } = renderPanel()

    await user.selectOptions(await screen.findByLabelText('选择模型'), 'c/third')
    await user.click(screen.getByRole('button', { name: '添加模型' }))
    expect(await screen.findByText('c/third')).toBeVisible()

    // Newly added models start enabled without resolutions: save stays
    // blocked until at least one badge is set.
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(await screen.findByText('启用的模型至少需要一个分辨率')).toBeVisible()
    expect(emitted().save).toBeUndefined()

    await user.click(screen.getByRole('button', { name: 'c/third1K分辨率未支持' }))
    await user.click(screen.getByRole('button', { name: '移除b/second' }))
    expect(screen.queryByText('b/second')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(emitted().save).toHaveLength(1)
    expect(emitted().save![0]).toEqual([
      [
        { modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
        { modelId: 'c/third', enabled: true, resolutions: ['1k'] },
      ],
    ])
  })

  it('reserves one error line per row so validation cannot shift rows', async () => {
    const { user } = renderPanel({
      entries: [{ modelId: 'a/first', enabled: true, resolutions: [] }],
    })

    // 错误行恒占位：无错误时留空盒，校验出现只填文本、不增删节点。
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveClass('min-h-4')
    expect(alert).toHaveTextContent('')

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(await screen.findByText('启用的模型至少需要一个分辨率')).toBeVisible()
  })

  it('shows mutation errors and offers reload on stale versions', async () => {
    renderPanel({ errorMessage: '保存模型池失败', stale: true })

    expect(await screen.findByText('保存模型池失败')).toBeVisible()
    expect(screen.getByRole('button', { name: '重新加载' })).toBeVisible()
  })

  it('blocks saving until the pool load succeeds and can retry catalog loading', async () => {
    const { user, emitted } = renderPanel({
      saveDisabled: true,
      catalogError: '加载模型目录失败，请重试',
    })

    expect(await screen.findByText('加载模型目录失败，请重试')).toBeVisible()
    await user.click(screen.getAllByRole('button', { name: '重新加载' })[0]!)
    expect(emitted().reloadCatalog).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(emitted().save).toBeUndefined()
  })
})
