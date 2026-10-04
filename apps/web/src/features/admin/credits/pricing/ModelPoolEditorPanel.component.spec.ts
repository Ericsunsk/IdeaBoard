import { defineComponent, h, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import ModelPoolEditorPanel from './ModelPoolEditorPanel.vue'
import type {
  AdminBillableItemCard,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
} from './billableCatalogTypes'

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
    version: { type: Number, default: 2 },
    entries: { type: Array as () => AdminModelPoolEntry[], default: () => entries },
    catalogModels: { type: Array as () => AdminPoolCatalogModel[], default: () => catalogModels },
    catalogLoading: { type: Boolean, default: false },
    pending: { type: Boolean, default: false },
    errorMessage: { type: String, default: '' },
    stale: { type: Boolean, default: false },
    saveDisabled: { type: Boolean, default: false },
    catalogError: { type: String, default: '' },
  },
  emits: ['submit', 'reload', 'reloadCatalog'],
  setup(props, { emit }) {
    const panelRef = ref<InstanceType<typeof ModelPoolEditorPanel> | null>(null)
    return () =>
      h('div', [
        h(ModelPoolEditorPanel, {
          ref: panelRef,
          ...props,
          onSubmit: (val: AdminModelPoolEntry[]) => emit('submit', val),
          onReload: () => emit('reload'),
          onReloadCatalog: () => emit('reloadCatalog'),
        }),
        h(
          'button',
          {
            type: 'button',
            onClick: () => {
              if (!props.saveDisabled) panelRef.value?.submit()
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
      version: 2,
      entries,
      catalogModels,
      ...overrides,
    },
  })
}

describe('ModelPoolEditorPanel', () => {
  it('renders entries with version and resolution badges', async () => {
    renderPanel()

    expect(await screen.findByText('a/first')).toBeVisible()
    expect(screen.getByText('b/second')).toBeVisible()
    expect(screen.getByText('版本 v2')).toBeVisible()
    // 1k badge appears once (a/first); 2k once; 4k toggle buttons exist per entry.
    expect(screen.getByRole('button', { name: 'a/first1K分辨率已支持' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'b/second1K分辨率未支持' })).toBeVisible()
    expect(screen.queryByText(/priority|fallback|weight/i)).not.toBeInTheDocument()
  })

  it('toggles a resolution badge and emits the updated draft on save', async () => {
    const { user, emitted } = renderPanel()

    await user.click(await screen.findByRole('button', { name: 'b/second4K分辨率未支持' }))
    expect(await screen.findByRole('button', { name: 'b/second4K分辨率已支持' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(emitted().submit).toHaveLength(1)
    expect(emitted().submit![0]).toEqual([
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
    expect(emitted().submit).toBeUndefined()
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
    expect(emitted().submit).toBeUndefined()

    await user.click(screen.getByRole('button', { name: 'c/third1K分辨率未支持' }))
    await user.click(screen.getByRole('button', { name: '移除b/second' }))
    expect(screen.queryByText('b/second')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(emitted().submit).toHaveLength(1)
    expect(emitted().submit![0]).toEqual([
      [
        { modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
        { modelId: 'c/third', enabled: true, resolutions: ['1k'] },
      ],
    ])
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
    expect(emitted().submit).toBeUndefined()
  })
})
