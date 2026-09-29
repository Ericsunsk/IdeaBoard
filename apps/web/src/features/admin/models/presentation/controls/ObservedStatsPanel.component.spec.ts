import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../../test/render'
import ObservedStatsPanel from './ObservedStatsPanel.vue'
import type { ModelObservedSnapshot } from '../../domain/catalogData'

function observed(overrides: Partial<ModelObservedSnapshot> = {}): ModelObservedSnapshot {
  return {
    model: 'openai/gpt-image-1',
    provider: 'OpenAI',
    windowDays: 7,
    windowSince: '2026-09-15T00:00:00Z',
    attemptsTotal: 100,
    completed: 92,
    failed: 8,
    cacheHits: 30,
    successRate: 0.92,
    cacheHitRate: 0.3,
    topFailureStage: 'render',
    topFailureCategory: 'timeout',
    topFailureCount: 5,
    hasObservations: true,
    hasSkuMargin: true,
    skuMargins: [
      {
        sku: 'ai-image:gpt-image-1',
        displayName: 'GPT Image 1',
        service: 'generation',
        onSale: true,
        unitCreditsByVariant: { '1k': 100 },
        actualCostCredits: null,
        hasActualCost: false,
        costSource: 'unavailable',
        costNote: '网关实际成本暂无来源',
        marginCredits: null,
        marginStatus: 'unknown_no_cost',
      },
    ],
    ...overrides,
  }
}

describe('ObservedStatsPanel', () => {
  it('renders success rate, top failure, cache hit rate and on-sale SKU markers', () => {
    render(ObservedStatsPanel, { props: { observed: observed() } })

    expect(screen.getByText('92%')).toBeInTheDocument()
    expect(screen.getByText('30%')).toBeInTheDocument()
    expect(screen.getByText('render / timeout · 5 次')).toBeInTheDocument()
    expect(screen.getByText('GPT Image 1 · 在售')).toBeInTheDocument()
    // margin/cost 恒 null：只展示暂无数据，绝不编造。
    expect(screen.getByText('成本 / 毛利')).toBeInTheDocument()
    expect(screen.getAllByText('暂无数据').length).toBeGreaterThan(0)
  })

  it('shows 暂无数据 for null rates and empty failures without fabricating', () => {
    render(ObservedStatsPanel, {
      props: {
        observed: observed({
          successRate: null,
          cacheHitRate: null,
          topFailureStage: '',
          topFailureCategory: '',
          topFailureCount: 0,
          skuMargins: [],
        }),
      },
    })

    expect(screen.getAllByText('暂无数据').length).toBeGreaterThanOrEqual(4)
    expect(screen.queryByText(/%/)).not.toBeInTheDocument()
  })

  it('marks off-sale SKUs distinctly from on-sale ones', () => {
    render(ObservedStatsPanel, {
      props: {
        observed: observed({
          skuMargins: [
            {
              sku: 'ai-image:off',
              displayName: 'Off Sale',
              service: 'generation',
              onSale: false,
              unitCreditsByVariant: {},
              actualCostCredits: null,
              hasActualCost: false,
              costSource: 'unavailable',
              costNote: '',
              marginCredits: null,
              marginStatus: 'unknown_no_cost',
            },
          ],
        }),
      },
    })

    expect(screen.getByText('SKU 状态')).toBeInTheDocument()
    expect(screen.getByText('Off Sale · 非在售')).toBeInTheDocument()
  })

  it('falls back gracefully without observations', () => {
    render(ObservedStatsPanel, { props: { observed: null } })
    expect(screen.getByText(/暂无 7 天观测数据/)).toBeInTheDocument()

    render(ObservedStatsPanel, {
      props: { observed: observed({ hasObservations: false }) },
    })
    expect(screen.getAllByText(/暂无 7 天观测数据/).length).toBeGreaterThan(0)
  })

  it('keeps layout while loading', () => {
    render(ObservedStatsPanel, { props: { observed: null, loading: true } })
    expect(screen.getByText('正在加载观测数据...')).toBeInTheDocument()
  })
})
