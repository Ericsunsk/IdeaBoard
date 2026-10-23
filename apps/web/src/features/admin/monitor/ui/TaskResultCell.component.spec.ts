import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '@/test/render'
import TaskResultCell from './TaskResultCell.vue'
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
    status: 'completed',
    error_msg: '',
    result: '',
    created_at: '2026-07-27T00:00:00Z',
    completed_at: '2026-07-27T00:01:00Z',
    ...overrides,
  } as AdminTaskRow
}

const urls = [
  '/api/v1/workspaces/ws_test123/media-assets/asset_a/content',
  '/api/v1/workspaces/ws_test123/media-assets/asset_b/content',
  '/api/v1/workspaces/ws_test123/media-assets/asset_c/content',
  '/api/v1/workspaces/ws_test123/media-assets/asset_d/content',
]

describe('TaskResultCell', () => {
  it('renders up to 3 thumbnails with an overflow badge', async () => {
    const view = renderWithUser(TaskResultCell, {
      props: { row: makeRow({ result: JSON.stringify({ image_urls: urls }) }) },
    })

    expect(screen.getAllByRole('button')).toHaveLength(3)
    expect(screen.getByText('+1')).toBeInTheDocument()

    await view.user.click(screen.getAllByRole('button')[0]!)
    expect(view.emitted()['inspect']?.[0]).toEqual([expect.objectContaining({ task_id: 'task-1' })])
  })

  it('renders a dash when there is no result', () => {
    renderWithUser(TaskResultCell, { props: { row: makeRow({ status: 'pending' }) } })

    expect(screen.getByText('-')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
