import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import TaskControlToolbar from './TaskControlToolbar.vue'
import TaskEventsCard from './TaskEventsCard.vue'
import TaskActivityTable from './TaskActivityTable.vue'
import { timeRangePresets } from '../../core/timeRange'
import type { TaskMetricStatsSnapshot } from '../../../shared'
import type { AdminTaskRow } from '../../session/useAdminTaskList'

describe('Monitor Components', () => {
  it('renders TaskControlToolbar and emits search on enter', async () => {
    const onSearch = vi.fn()
    const { user } = renderWithUser(TaskControlToolbar, {
      props: {
        searchKeyword: 'prompt text',
        selectedPreset: timeRangePresets[0]!,
        selectedTimeRangeKey: '1h',
        timeRangePresets,
        onSearch,
      },
    })

    const input = screen.getByRole('textbox', { name: /Search|搜索/ })
    expect(input).toHaveValue('prompt text')
    await user.type(input, '{enter}')
    expect(onSearch).toHaveBeenCalled()
  })

  it('renders MonitorEventsCard and toggles charts collapsed', async () => {
    const onUpdateCollapse = vi.fn()
    const onToggleStatus = vi.fn()
    const stats: TaskMetricStatsSnapshot = {
      total: 10,
      pending: 1,
      running: 2,
      completed: 5,
      failed: 1,
      cancelled: 1,
    }

    const { user } = renderWithUser(TaskEventsCard, {
      props: {
        filterStatus: null,
        metricStatsSnapshot: stats,
        chartsCollapsed: false,
        timelineChartData: { labels: [], datasets: [] },
        timelineChartOptions: {} as never,
        adminChartPlugins: [],
        chartHeightPx: 120,
        'onUpdate:chartsCollapsed': onUpdateCollapse,
        onToggleFilterStatus: onToggleStatus,
      },
    })

    expect(screen.getByText('Events')).toBeVisible()
    const collapseBtn = screen.getByRole('button', { name: '折叠图表' })
    await user.click(collapseBtn)
    expect(onUpdateCollapse).toHaveBeenCalledWith(true)

    const runningPill = screen.getByTitle('Filter running tasks')
    // 过滤 pills 中性分类：激活态走 bg-muted 载体，不带 success/warning/info/destructive 状态色。
    const runningDot = runningPill.querySelector('span:first-child')!
    expect(runningDot.className).toMatch(/bg-muted-foreground/)
    expect(runningDot.className).not.toMatch(/bg-warning|bg-success|bg-info|bg-destructive/)
    await user.click(runningPill)
    expect(onToggleStatus).toHaveBeenCalledWith('running')
  })

  it('renders active filter pill with neutral classification style', async () => {
    const stats: TaskMetricStatsSnapshot = {
      total: 10,
      pending: 1,
      running: 2,
      completed: 5,
      failed: 1,
      cancelled: 1,
    }

    renderWithUser(TaskEventsCard, {
      props: {
        filterStatus: 'failed',
        metricStatsSnapshot: stats,
        chartsCollapsed: false,
        timelineChartData: { labels: [], datasets: [] },
        timelineChartOptions: {} as never,
        adminChartPlugins: [],
        chartHeightPx: 120,
      },
    })

    const failedPill = screen.getByTitle('Filter failed tasks')
    expect(failedPill.className).toMatch(/bg-muted/)
    expect(failedPill.className).not.toMatch(/bg-destructive-soft|text-destructive/)
  })

  it('renders TaskActivityTable with tasks and pagination', async () => {
    const onShowDetail = vi.fn()
    const onPrev = vi.fn()
    const mockRow: AdminTaskRow = {
      task_id: 't-123',
      user_public_id: 'u-1',
      user_display_name: 'Tester',
      prompt: 'hello test prompt',
      priority: 1,
      status: 'completed',
      error_msg: '',
      result: 'res',
      service: 'image_generate',
      created_at: '2026-09-20T10:00:00Z',
      completed_at: '2026-09-20T10:00:05Z',
      credits: 10,
    }

    const { user } = renderWithUser(TaskActivityTable, {
      props: {
        tasks: [mockRow],
        loading: false,
        totalCount: 1,
        currentPage: 2,
        totalPages: 5,
        canPrevious: true,
        canNext: true,
        canCancelTask: () => false,
        isCancellingTask: () => false,
        onShowDetail,
        onPrevPage: onPrev,
      },
    })

    expect(screen.getByText('hello test prompt')).toBeVisible()
    expect(screen.getByText('共 1 条记录')).toBeVisible()

    const detailBtn = screen.getByLabelText('更多任务详情')
    await user.click(detailBtn)
    expect(onShowDetail).toHaveBeenCalledWith(mockRow)

    const prevBtn = screen.getByRole('button', { name: '上一页' })
    await user.click(prevBtn)
    expect(onPrev).toHaveBeenCalled()
  })
})
