import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import AdminTaskResultDialog from './AdminTaskResultDialog.vue'
import { adminTaskStatuses } from './taskMonitorStatus'
import type {
  AdminTaskMonitorDetailModel,
  AdminTaskMonitorExecutionAttempt,
} from './taskMonitorResource'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}))

vi.mock('../shared/lib/adminToast', () => ({
  useAdminToast: () => toast,
}))

function renderDialog(task: AdminTaskMonitorDetailModel, visible = true) {
  return renderWithUser(AdminTaskResultDialog, {
    props: { task, visible },
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: { transition: false },
    },
  })
}

describe('AdminTaskResultDialog', () => {
  it.each([
    {
      label: 'completed',
      task: { status: adminTaskStatuses.completed },
      statusText: '已完成',
      detailText: undefined,
    },
    {
      label: 'failed',
      task: { status: adminTaskStatuses.failed, error_msg: '上游请求失败' },
      statusText: '失败',
      detailText: '上游请求失败',
    },
    {
      label: 'cancelled',
      task: { status: adminTaskStatuses.cancelled },
      statusText: '已停止',
      detailText: '任务已停止，未继续执行。',
    },
  ] satisfies ReadonlyArray<{
    label: string
    task: Partial<AdminTaskMonitorDetailModel>
    statusText: string
    detailText?: string
  }>)('renders the $label task state through the shared status definition', async (testCase) => {
    renderDialog(makeTask(testCase.task))

    expect(await screen.findByRole('dialog', { name: '任务详情控制台' })).toBeInTheDocument()
    expect(screen.getAllByText(testCase.statusText)).not.toHaveLength(0)
    if (testCase.detailText) expect(screen.getByText(testCase.detailText)).toBeInTheDocument()
  })

  it('shows an explicit empty-result state', async () => {
    renderDialog(makeTask({ result: undefined }))

    expect(await screen.findByText('没有可预览的图片结果。')).toBeInTheDocument()
    expect(screen.getByText('暂无 API 响应结果')).toBeInTheDocument()
  })

  it('uses one empty marker when there is no active assignment', async () => {
    renderDialog(makeTask())

    expect(await screen.findByText('身份与归属')).toBeVisible()
    expect(screen.queryByText('N/A')).not.toBeInTheDocument()
    expect(screen.queryByText(/Endpoint #/)).not.toBeInTheDocument()
  })

  it('rejects unsafe result images without rendering them', async () => {
    renderDialog(makeTask({ result: '{"image_url":"javascript:alert(1)"}' }))

    expect(await screen.findByText('结果图片链接不安全，已拒绝预览。')).toBeInTheDocument()
    expect(document.querySelector('img[src*="javascript"]')).toBeNull()
  })

  it('clears debug expansion after closing and reopening', async () => {
    const { rerender, user } = renderDialog(makeTask({ result: '{"status":"completed"}' }))
    const expandButton = await screen.findByRole('button', {
      name: '原始调试详情 (脱敏) · 展开',
    })
    expect(expandButton).toHaveAttribute('aria-expanded', 'false')
    expect(expandButton).toHaveAttribute('aria-controls', 'task-result-debug-details')

    await user.click(expandButton)

    expect(screen.getByRole('button', { name: '原始调试详情 (脱敏) · 隐藏' })).toHaveAttribute(
      'aria-expanded',
      'true',
    )
    expect(document.getElementById('task-result-debug-details')).toBeVisible()

    await rerender({ visible: false })
    await rerender({ visible: true })

    expect(
      await screen.findByRole('button', { name: '原始调试详情 (脱敏) · 展开' }),
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('keeps the result action visible for keyboard focus', async () => {
    renderDialog(
      makeTask({
        result: '{"image_url":"/uploads/tasks/1/task/output.png"}',
      }),
    )

    const openButton = await screen.findByRole('button', { name: '在新窗口打开' })
    expect(openButton).toHaveClass('group-focus-within:!opacity-100', 'focus-visible:!opacity-100')
  })

  it('marks an immutable Admin Billing snapshot when its feature no longer exists', async () => {
    renderDialog(
      makeTask({
        billing: {
          sku: 'legacy:removed',
          display_name: '已移除功能',
          price_version_id: 9,
          price_version: 2,
          unit_credits: 3,
          quantity: 2,
          total_credits: 6,
          quoted_at: '2026-07-15T08:00:00Z',
          settlement_state: 'released',
          settled_at: '2026-07-15T08:01:00Z',
          feature_exists: false,
        },
      }),
    )

    expect(await screen.findByText('计费快照')).toBeVisible()
    expect(screen.getByText('功能已删除')).toBeVisible()
    expect(screen.getByText('已移除功能')).toBeVisible()
    expect(screen.getByText('legacy:removed')).toBeVisible()
    expect(screen.getByText('6 Ideas')).toBeVisible()
    expect(screen.getByText('已释放')).toBeVisible()
  })

  it('renders ordered retry and failover attempts without exposing credentials', async () => {
    renderDialog(
      makeTask({
        execution_attempts: [
          makeAttempt({
            attempt_no: 1,
            status: 'requeued',
            stage: 'provider',
            failure_category: 'timeout',
            provider_endpoint_id: 11,
            provider_endpoint_name: 'Provider A',
            resolved_model: 'model-a',
            total_latency_ms: 1800,
          }),
          makeAttempt({
            attempt_no: 2,
            status: 'completed',
            stage: 'completed',
            provider_endpoint_id: 12,
            provider_endpoint_name: 'Provider B',
            resolved_model: 'model-b',
            total_latency_ms: 900,
          }),
        ],
      }),
    )

    const firstProvider = await screen.findByText('Provider A')
    const secondProvider = screen.getByText('Provider B')
    expect(
      firstProvider.compareDocumentPosition(secondProvider) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(screen.getByText('model-a')).toBeVisible()
    expect(screen.getByText('model-b')).toBeVisible()
    expect(screen.getByText('超时')).toBeVisible()
    expect(screen.queryByText('Credential')).not.toBeInTheDocument()
  })

  it('uses the backend latest execution snapshot for the detail header', async () => {
    renderDialog(
      makeTask({
        latest_execution: {
          account_id: 99,
          provider_endpoint_id: 12,
          provider_endpoint_name: 'Header Endpoint',
          resolved_model: 'header-model',
          api_type: 'google-native',
        },
        execution_attempts: [
          makeAttempt({
            provider_endpoint_name: 'Trace Endpoint',
            resolved_model: 'trace-model',
          }),
        ],
      }),
    )

    expect(await screen.findByText('Header Endpoint')).toBeVisible()
    expect(screen.getByText('header-model · google-native')).toBeVisible()
    expect(screen.getByText('99')).toBeVisible()
    expect(screen.getByText('Trace Endpoint')).toBeVisible()
  })

  it('renders explicit loading, error, and empty trace states', async () => {
    const retry = vi.fn()
    const { rerender, user } = renderWithUser(AdminTaskResultDialog, {
      props: {
        task: null,
        taskId: 'task_1',
        visible: true,
        loading: true,
        onRetry: retry,
      },
      global: {
        plugins: [[PrimeVue, primeVueTestOptions]],
        stubs: { transition: false },
      },
    })

    expect(await screen.findByText('正在加载任务详情…')).toBeVisible()

    await rerender({ loading: false, errorMessage: '加载任务详情失败' })
    await user.click(await screen.findByRole('button', { name: '重试' }))
    expect(retry).toHaveBeenCalledOnce()

    await rerender({ task: makeTask(), errorMessage: '' })
    expect(await screen.findByText('暂无执行记录')).toBeVisible()
  })
})

function makeAttempt(
  overrides: Partial<AdminTaskMonitorExecutionAttempt> = {},
): AdminTaskMonitorExecutionAttempt {
  return {
    attempt_no: 1,
    account_id: 1,
    status: 'completed',
    stage: 'completed',
    failure_category: '',
    provider_endpoint_id: 1,
    provider_endpoint_name: 'Provider',
    resolved_model: 'model',
    api_type: 'google-native',
    provider_started_at: '2026-07-06T00:00:00Z',
    provider_finished_at: '2026-07-06T00:00:01Z',
    finalize_started_at: null,
    finalize_finished_at: null,
    completed_at: '2026-07-06T00:00:01Z',
    cancelled_at: null,
    provider_latency_ms: 800,
    finalize_latency_ms: 0,
    total_latency_ms: 1000,
    ...overrides,
  }
}

function makeTask(
  overrides: Partial<AdminTaskMonitorDetailModel> = {},
): AdminTaskMonitorDetailModel {
  return {
    task_id: 'task_1',
    correlation_id: 'task_1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'Prompt',
    user_prompt: '',
    params: '{}',
    feature_slug: 'sketch-to-image',
    priority: 2,
    status: adminTaskStatuses.completed,
    retry_count: 0,
    error_msg: '',
    result: '',
    created_at: '2026-07-06T00:00:00Z',
    updated_at: '2026-07-06T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    cancel_reason: '',
    execution_attempts: [],
    ...overrides,
  }
}
