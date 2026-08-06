import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen } from '../../../../test/render'
import BillingPriceHistoryDialog from './BillingPriceHistoryDialog.vue'

const item = {
  sku: 'ai-image:partial-edit',
  namespace: 'ai-image',
  productKind: 'workflow-feature',
  displayName: '局部重绘',
  description: '仅重绘图片选定区域。',
  sortOrder: 10,
  status: 'active' as const,
  currentPrice: {
    priceVersionId: 2,
    version: 2,
    unitCredits: 4,
    createdAt: '2026-07-16T08:00:00Z',
  },
}

function renderHistory(props: Record<string, unknown>) {
  return renderWithUser(BillingPriceHistoryDialog, {
    props: { visible: true, item, ...props },
    global: { plugins: [[PrimeVue, primeVueTestOptions]], stubs: { transition: false } },
  })
}

describe('BillingPriceHistoryDialog', () => {
  it('renders the loading state', async () => {
    renderHistory({ loading: true })
    expect(await screen.findByText('正在加载价格历史')).toBeInTheDocument()
  })

  it('renders the retryable error state', async () => {
    renderHistory({ errorMessage: 'network unavailable' })
    expect(await screen.findByText('价格历史加载失败')).toBeVisible()
    expect(screen.getByRole('button', { name: '重新加载' })).toBeVisible()
  })

  it('renders the empty state', async () => {
    renderHistory({ prices: [] })
    expect(await screen.findByText('暂无价格历史')).toBeVisible()
    expect(screen.getByText('发布首个价格后，价格历史会显示在这里。')).toBeVisible()
  })

  it('marks only the current immutable price version', async () => {
    renderHistory({
      prices: [
        item.currentPrice,
        { ...item.currentPrice, priceVersionId: 1, version: 1, unitCredits: 2 },
      ],
      hasMore: true,
    })

    expect(await screen.findByText('版本 2')).toBeVisible()
    expect(screen.getByText('版本 1')).toBeVisible()
    expect(screen.getAllByText('当前版本')).toHaveLength(1)
    expect(screen.getByRole('button', { name: '加载更多' })).toBeVisible()
  })
})
