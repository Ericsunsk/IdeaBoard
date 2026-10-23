import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import TaskStatusCell from './TaskStatusCell.vue'
import type { AdminTaskRow } from '../session/useAdminTaskList'

function makeRow(overrides: Partial<AdminTaskRow> = {}): AdminTaskRow {
  return {
    task_id: 'task-1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'prompt',
    service: 'render',
    credits: 0,
    priority: 2,
    status: 'running',
    error_msg: '',
    result: '',
    created_at: '2026-07-27T00:00:00Z',
    completed_at: null,
    ...overrides,
  } as AdminTaskRow
}

describe('TaskStatusCell', () => {
  it('renders the metric pill for known statuses', () => {
    renderWithUser(TaskStatusCell, { props: { row: makeRow({ status: 'failed' }) } })

    // 方案 A：载体统一 StatusBadge，文案收敛中文（core/status 单源）。
    expect(screen.getByText('失败')).toBeInTheDocument()
  })

  it('falls back to raw status text for unknown statuses', () => {
    renderWithUser(TaskStatusCell, { props: { row: makeRow({ status: 'mystery' }) } })

    expect(screen.getByText('mystery')).toBeInTheDocument()
  })
})
