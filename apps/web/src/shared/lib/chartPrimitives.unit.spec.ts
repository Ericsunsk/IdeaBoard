import { describe, expect, it, vi } from 'vitest'
import {
  createChartEmptyStatePlugin,
  defineChartNumericScale,
  defineChartOptions,
  getParsedY,
  type ChartTooltipItem,
  withAlpha,
} from './chartPrimitives'

describe('chart primitives', () => {
  it('draws the shared empty state only when every chart datum is zero', () => {
    const fillText = vi.fn()
    const context = {
      save: vi.fn(),
      restore: vi.fn(),
      fillText,
      fillStyle: '',
      font: '',
      textAlign: '',
      textBaseline: '',
    }
    const plugin = createChartEmptyStatePlugin({
      id: 'testEmptyState',
      textColor: '#667085',
      fontFamily: 'system-ui',
    })
    const afterDraw = plugin.afterDraw as (chart: unknown) => void

    afterDraw({
      data: { datasets: [{ data: [0, 0] }] },
      ctx: context,
      chartArea: { left: 0, right: 100, top: 0, bottom: 50 },
    })
    expect(fillText).toHaveBeenCalledWith('No data', 50, 25)

    fillText.mockClear()
    afterDraw({
      data: { datasets: [{ data: [0, 1] }] },
      ctx: context,
      chartArea: { left: 0, right: 100, top: 0, bottom: 50 },
    })
    expect(fillText).not.toHaveBeenCalled()
  })

  it('builds a numeric scale from injected colors and tick formatting', () => {
    const scale = defineChartNumericScale({
      values: [0, 9],
      min: 10,
      headroom: 1.15,
      stacked: true,
      tickColor: '#445566',
      gridColor: '#ddeeff',
      formatTick: (value) => `${value.toLocaleString('en-US')} credits`,
    })

    expect(scale).toMatchObject({
      stacked: true,
      beginAtZero: true,
      max: 15,
      ticks: {
        color: '#445566',
      },
      grid: { color: '#ddeeff' },
      border: { display: false },
    })
    expect(scale.ticks.callback(1_200)).toBe('1,200 credits')
  })

  it('applies the injected font family without replacing local typography', () => {
    const options = defineChartOptions(
      {
        plugins: {
          legend: { labels: { font: { size: 11, weight: 600 } } },
          tooltip: { bodyFont: { size: 12 }, titleFont: { weight: 700 } },
        },
        scales: {
          x: {
            title: { display: true },
            ticks: { font: { size: 10 } },
          },
        },
      },
      { fontFamily: 'Inter, sans-serif', hideScaleTitles: true },
    )
    const typography = options as {
      plugins: {
        legend: { labels: { font: { family: string; size: number; weight: number } } }
        tooltip: {
          bodyFont: { family: string; size: number }
          titleFont: { family: string; weight: number }
        }
      }
      scales: {
        x: {
          title: { display: boolean }
          ticks: { font: { family: string; size: number } }
        }
      }
    }

    expect(typography.plugins.legend.labels.font).toEqual({
      family: 'Inter, sans-serif',
      size: 11,
      weight: 600,
    })
    expect(typography.plugins.tooltip.bodyFont).toEqual({
      family: 'Inter, sans-serif',
      size: 12,
    })
    expect(typography.plugins.tooltip.titleFont).toEqual({
      family: 'Inter, sans-serif',
      weight: 700,
    })
    expect(typography.scales.x).toMatchObject({
      title: { display: false },
      ticks: { font: { family: 'Inter, sans-serif', size: 10 } },
    })
  })

  it('parses tooltip values and applies alpha to full or shorthand hex colors', () => {
    expect(getParsedY({ parsed: { y: 4.5 }, raw: 99 } as unknown as ChartTooltipItem)).toBe(4.5)
    expect(getParsedY({ parsed: 7 } as unknown as ChartTooltipItem)).toBe(7)
    expect(getParsedY({ parsed: {}, raw: '12' } as unknown as ChartTooltipItem)).toBe(12)
    expect(withAlpha('#336699', 0.25)).toBe('rgba(51, 102, 153, 0.25)')
    expect(withAlpha('#abc', 0.5)).toBe('rgba(170, 187, 204, 0.5)')
  })
})
