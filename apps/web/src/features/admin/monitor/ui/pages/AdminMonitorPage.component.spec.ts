import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import AdminMonitorPage from './AdminMonitorPage.vue'

const mocks = vi.hoisted(() => ({
  requestCancelTask: vi.fn(),
}))

const mockFilterStatus = ref<string | null>(null)

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

const mockTasks = ref<Array<typeof task>>([task])

vi.mock('../../session/useAdminTaskList', () => ({
  useAdminTaskList: () => ({
    tasks: mockTasks,
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
    filterStatus: mockFilterStatus,
    searchKeyword: ref(''),
    appliedSearchKeyword: ref(''),
    loadTasks: vi.fn(),
    handleSearch: vi.fn(),
  }),
}))

vi.mock('../../session/useAdminTaskRuntimeSnapshot', () => ({
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

vi.mock('../../resource', () => ({
  useAdminTaskDetailResource: () => ({
    data: ref(undefined),
    isError: ref(false),
    refetch: vi.fn(),
  }),
  useAdminTaskListResource: () => ({
    data: ref({ list: [] }),
  }),
}))

vi.mock('../../session/useAdminTaskCancel', () => ({
  useAdminTaskCancel: () => ({
    isCancellableTask: (row: typeof task) => row.status === 'pending' || row.status === 'running',
    isCancellingTask: () => false,
    requestCancelTask: mocks.requestCancelTask,
  }),
}))

vi.mock('../../session/useAdminMonitorFeedback', () => ({
  useAdminTaskCancellationFeedback: () => ({ taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }),
  useAdminTaskLoadFeedback: () => ({
    snapshotLoadFailed: vi.fn(),
    listLoadFailed: vi.fn(),
    detailLoadFailed: vi.fn(),
  }),
}))

vi.mock('../../../shared/session/feedback/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
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

describe('AdminMonitorPage', () => {
  beforeEach(() => {
    mockFilterStatus.value = null
    mockTasks.value = [task]
    mocks.requestCancelTask.mockClear()
  })

  it('shows one-row cancellation only for an active task and delegates to the cancel command', async () => {
    mocks.requestCancelTask.mockClear()
    mockFilterStatus.value = null
    const view = renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    await view.user.click(screen.getByLabelText('取消任务'))

    expect(mocks.requestCancelTask).toHaveBeenCalledWith(task)
  })

  it('filters task status when clicking observability status pills', async () => {
    mockFilterStatus.value = null
    const view = renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    await view.user.click(screen.getByTitle('Filter pending tasks'))
    expect(mockFilterStatus.value).toBe('pending')

    await view.user.click(screen.getByTitle('Filter running tasks'))
    expect(mockFilterStatus.value).toBe('running')

    await view.user.click(screen.getByTitle('Filter completed tasks'))
    expect(mockFilterStatus.value).toBe('completed')

    await view.user.click(screen.getByTitle('Filter failed tasks'))
    expect(mockFilterStatus.value).toBe('failed')

    await view.user.click(screen.getByTitle('Filter failed tasks'))
    expect(mockFilterStatus.value).toBeNull()

    await view.user.click(screen.getByTitle('Filter cancelled tasks'))
    expect(mockFilterStatus.value).toBe('cancelled')

    await view.user.click(screen.getByTitle('Filter cancelled tasks'))
    expect(mockFilterStatus.value).toBeNull()

    await view.user.click(screen.getByTitle('Show all tasks'))
    expect(mockFilterStatus.value).toBeNull()
  })

  it('toggles chart visibility when clicking the collapse chart button', async () => {
    const view = renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    const collapseButton = screen.getByLabelText('折叠图表')
    expect(collapseButton).toBeInTheDocument()

    await view.user.click(collapseButton)
    expect(screen.getByLabelText('展开图表')).toBeInTheDocument()
  })

  it('renders the resolved time range on the picker trigger', () => {
    renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    // The picker is a real reka `Select`, whose option list renders into a portal
    // that only opens on real pointer input. jsdom therefore sees the trigger and
    // nothing else, so open-state and keyboard behaviour live in
    // TaskControlToolbar.browser.spec.ts; what the page owns is that the
    // resolved preset reaches the trigger as its value.
    const trigger = screen.getByRole('combobox', { name: '选择时间范围' })
    expect(trigger).toHaveTextContent('过去 1 小时')
  })

  it('renders table columns directly as the header without redundant Activity Log title', () => {
    renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    // 确认没有多余的 Activity Log 标题
    expect(screen.queryByText('Activity Log')).not.toBeInTheDocument()
    // 确认渲染了表头字段
    expect(screen.getByText('时间戳')).toBeInTheDocument()
    expect(screen.getByText('Duration')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(screen.getByText('Service')).toBeInTheDocument()
    expect(screen.getByText('Model')).toBeInTheDocument()
    expect(screen.getByText('Message')).toBeInTheDocument()
    expect(screen.getByText('Credits')).toBeInTheDocument()
    expect(screen.getByText('Result')).toBeInTheDocument()
    // 确认底栏记录总数
    expect(screen.getByText(/共 \d+ 条记录/)).toBeInTheDocument()
  })

  it('renders the empty state when the task list is empty', async () => {
    mockTasks.value = []
    renderWithUser(AdminMonitorPage, {
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
          Refresh: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    expect(screen.getByText('暂无事件日志')).toBeInTheDocument()
  })
})
