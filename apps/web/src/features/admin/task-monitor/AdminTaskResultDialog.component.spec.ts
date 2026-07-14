import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
import type { Task } from '../../../shared/api/task'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import AdminTaskResultDialog from './AdminTaskResultDialog.vue'
import { adminTaskStatuses } from './taskMonitorStatus'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}))

vi.mock('../shared/lib/adminToast', () => ({
  useAdminToast: () => toast,
}))

function renderDialog(task: Task, visible = true) {
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
    task: Partial<Task>
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
})

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    task_id: 'task_1',
    correlation_id: 'task_1',
    user_id: 1,
    prompt: 'Prompt',
    priority: 2,
    status: adminTaskStatuses.completed,
    created_at: '2026-07-06T00:00:00Z',
    updated_at: '2026-07-06T00:00:00Z',
    ...overrides,
  }
}
