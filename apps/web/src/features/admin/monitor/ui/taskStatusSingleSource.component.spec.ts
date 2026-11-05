import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import TaskStatusCell from './TaskStatusCell.vue'
import { getTaskStatusDefinition } from '../core/status'
import { getAttemptStatusLabel, getAttemptStatusTone } from './taskResultLabels'
import type { AdminTaskRow } from '../session/useAdminTaskList'

// 表格 TaskStatusCell 与弹窗 attempt 徽标同一 status 必须同文（五态全覆盖）：
// 两处文案/tone 均出自 core/status 单源，防止表格 EN、弹窗 ZH 再分叉。
const TASK_STATUSES = ['pending', 'running', 'completed', 'failed', 'cancelled'] as const

function makeRow(status: string): AdminTaskRow {
  return {
    task_id: 'task-1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'prompt',
    service: 'render',
    credits: 0,
    priority: 2,
    status,
    error_msg: '',
    result: '',
    created_at: '2026-07-27T00:00:00Z',
    completed_at: null,
  } as AdminTaskRow
}

describe('task status single source', () => {
  it.each(TASK_STATUSES)('TaskStatusCell and attempt labels share the %s copy', (status) => {
    const definition = getTaskStatusDefinition(status)

    expect(getAttemptStatusLabel(status)).toBe(definition.text)
    expect(getAttemptStatusTone(status)).toBe(definition.tone)

    renderWithUser(TaskStatusCell, { props: { row: makeRow(status) } })
    expect(screen.getByText(definition.text)).toBeInTheDocument()
  })
})
