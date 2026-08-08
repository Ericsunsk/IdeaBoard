/* eslint-disable vue/one-component-per-file -- Local stubs keep this page interaction test focused. */
import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import AdminTaskMonitorPage from './AdminTaskMonitorPage.vue'

const mocks = vi.hoisted(() => ({
  requestCancelTask: vi.fn(),
}))

const task = {
  task_id: 'task-running',
  user_public_id: '00000000-0000-0000-0000-000000000042',
  user_display_name: 'User 42',
  prompt: 'prompt',
  priority: 2,
  status: 'running',
  error_msg: '',
  result: '',
  created_at: '2026-07-29T00:00:00Z',
  completed_at: null,
}

vi.mock('../../task-monitor/useAdminTaskList', () => ({
  useAdminTaskList: () => ({
    tasks: ref([task]),
    total: ref(1),
    loading: ref(false),
    isError: ref(false),
    error: ref(null),
    pagination: {
      currentPage: ref(1),
      totalPages: ref(1),
      canPrevious: ref(false),
      canNext: ref(false),
      previous: vi.fn(),
      next: vi.fn(),
    },
    resultPreviewEnabled: ref(false),
    taskStats: ref({ pending: 0, running: 1, completed: 0, failed: 0, cancelled: 0, other: 0 }),
    filterStatus: ref(null),
    accountIdFilter: ref(null),
    searchKeyword: ref(''),
    loadTasks: vi.fn(),
    handleSearch: vi.fn(),
  }),
}))

vi.mock('../../task-monitor/useAdminTaskRuntimeSnapshot', () => ({
  useAdminTaskRuntimeSnapshot: () => ({
    monitorData: ref({ global_queue_len: 0, wait_queue_len: 0, running_attempts: 1 }),
    monitorStats: ref({}),
    monitorStreamStatus: ref('connected'),
    isStale: ref(false),
    isError: ref(false),
    error: ref(null),
    retry: vi.fn(),
  }),
}))

vi.mock('../../task-monitor/useTaskMonitorCharts', () => ({
  useTaskMonitorCharts: () => ({
    taskStatusSnapshot: ref({ failed: 0 }),
    statusTotalCount: ref(1),
    accountLoadRows: ref([]),
    accountLoadCurrent: ref(0),
    accountLoadCapacity: ref(0),
    peakAccountLoadPercent: ref(0),
    taskDistributionRows: ref([]),
    activeExecutionPressurePoints: ref([]),
    accountConcurrencyChartData: ref({}),
    accountConcurrencyChartOptions: ref({}),
    taskDistributionChartData: ref({}),
    taskDistributionChartOptions: ref({}),
    executionPressureChartData: ref({}),
    executionPressureChartOptions: ref({}),
  }),
}))

vi.mock('../../task-monitor/taskMonitorResource', () => ({
  useAdminTaskDetailResource: () => ({
    data: ref(undefined),
    isError: ref(false),
    refetch: vi.fn(),
  }),
}))

vi.mock('../../task-monitor/useAdminTaskCancel', () => ({
  useAdminTaskCancel: () => ({
    isCancellableTask: (row: typeof task) => row.status === 'pending' || row.status === 'running',
    isCancellingTask: () => false,
    requestCancelTask: mocks.requestCancelTask,
  }),
}))

vi.mock('../../task-monitor/useAdminTaskMonitorFeedback', () => ({
  useAdminTaskCancellationFeedback: () => ({ taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }),
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: vi.fn() }),
}))

const SlotStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const TablePanelStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', [slots.toolbar?.(), slots.default?.()])
  },
})

const ToolbarStub = defineComponent({
  setup(_, { slots }) {
    return () =>
      h('div', [
        slots.search?.(),
        slots.metrics?.(),
        slots.filters?.(),
        slots.actions?.({ iconButtonClass: '' }),
        slots.pager?.(),
      ])
  },
})

const DataTableStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const ColumnStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.body?.({ data: task }))
  },
})

const ButtonStub = defineComponent({
  inheritAttrs: false,
  props: { label: { type: String, default: '' }, loading: Boolean, disabled: Boolean },
  setup(props, { attrs, slots }) {
    return () =>
      h(
        'button',
        {
          ...attrs,
          disabled: props.disabled || props.loading,
        },
        [props.label, slots.icon?.()],
      )
  },
})

describe('AdminTaskMonitorPage', () => {
  it('shows one-row cancellation only for an active task and delegates to the cancel command', async () => {
    mocks.requestCancelTask.mockClear()
    const view = renderWithUser(AdminTaskMonitorPage, {
      global: {
        stubs: {
          AdminChartCard: SlotStub,
          AdminFilterPopover: SlotStub,
          AdminPagerControls: SlotStub,
          AdminResourceToolbar: ToolbarStub,
          AdminSummaryCard: SlotStub,
          AdminTablePanel: TablePanelStub,
          AdminTaskResultDialog: SlotStub,
          AdminToolbarSearch: SlotStub,
          Button: ButtonStub,
          Chart: SlotStub,
          Column: ColumnStub,
          DataTableFrame: DataTableStub,
          Eye: SlotStub,
          InputNumber: SlotStub,
          Message: SlotStub,
          PrimeImage: SlotStub,
          Refresh: SlotStub,
          Select: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    await view.user.click(screen.getByLabelText('取消任务'))

    expect(mocks.requestCancelTask).toHaveBeenCalledWith(task)
  })
})
