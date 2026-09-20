import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminMonitorPage from './AdminMonitorPage.vue'

const mocks = vi.hoisted(() => ({
  requestCancelTask: vi.fn(),
}))

const routerMocks = vi.hoisted(() => ({
  replace: vi.fn(),
  route: {
    path: '/admin/monitor',
    query: {} as Record<string, string | undefined>,
  },
}))

vi.mock('vue-router', async (importOriginal) => {
  const original = await importOriginal<typeof import('vue-router')>()
  return {
    ...original,
    useRoute: () => routerMocks.route,
    useRouter: () => ({ push: vi.fn(), replace: routerMocks.replace }),
  }
})

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

vi.mock('../../application/useAdminTaskList', () => ({
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

vi.mock('../../application/useAdminTaskRuntimeSnapshot', () => ({
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

vi.mock('../../infrastructure/resource', () => ({
  useAdminTaskDetailResource: () => ({
    data: ref(undefined),
    isError: ref(false),
    refetch: vi.fn(),
  }),
  useAdminTaskListResource: () => ({
    data: ref({ list: [] }),
  }),
}))

vi.mock('../../application/useAdminTaskCancel', () => ({
  useAdminTaskCancel: () => ({
    isCancellableTask: (row: typeof task) => row.status === 'pending' || row.status === 'running',
    isCancellingTask: () => false,
    requestCancelTask: mocks.requestCancelTask,
  }),
}))

vi.mock('../../application/useAdminMonitorFeedback', () => ({
  useAdminTaskCancellationFeedback: () => ({ taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }),
}))

vi.mock('../../../shared/composables/useAdminConfirm', () => ({
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
    routerMocks.route.query = {}
    routerMocks.replace.mockClear()
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
          Select: SlotStub,
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
          Select: SlotStub,
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
          Select: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    const collapseButton = screen.getByLabelText('折叠图表')
    expect(collapseButton).toBeInTheDocument()

    await view.user.click(collapseButton)
    expect(screen.getByLabelText('展开图表')).toBeInTheDocument()
  })

  it('renders time range picker and updates selection', async () => {
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
          Select: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    const trigger = screen.getByLabelText('选择时间范围')
    expect(trigger).toHaveTextContent('过去 1 小时')

    await view.user.click(trigger)
    const option24h = screen.getByRole('option', { name: /过去 24 小时/ })
    expect(option24h).toBeInTheDocument()

    await view.user.click(option24h)
    expect(trigger).toHaveTextContent('过去 24 小时')
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
          Select: SlotStub,
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

  it('renders the empty state without demo data when the task list is empty', async () => {
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
          Select: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    expect(screen.getByText('暂无事件日志')).toBeInTheDocument()
    expect(screen.queryByText('退出演示')).not.toBeInTheDocument()
    expect(screen.queryByText(/产品主图草图成图/)).not.toBeInTheDocument()
  })

  it('shows demo data with disabled actions only when ?demo=1 is present', async () => {
    mockTasks.value = []
    routerMocks.route.query = { demo: '1' }
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
          Select: SlotStub,
          Tag: SlotStub,
        },
      },
    })

    expect(screen.getByText(/共 42 条记录/)).toBeInTheDocument()
    expect(screen.getAllByText(/产品主图草图成图/).length).toBeGreaterThan(0)
    // Service 列展示 feature slug
    expect(screen.getAllByText('render').length).toBeGreaterThan(0)
    expect(screen.queryByText('暂无事件日志')).not.toBeInTheDocument()
    // 写操作在演示模式下禁用：无取消按钮；More 详情可用本地预览
    expect(screen.queryByLabelText('取消任务')).not.toBeInTheDocument()
    expect(screen.getAllByLabelText('更多任务详情').length).toBeGreaterThan(0)
  })
})
