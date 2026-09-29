import { describe, expect, it } from 'vitest'
import type { AdminMonitorListItemModel } from '../infrastructure/resource'
import {
  TASK_MESSAGE_MAX_LENGTH,
  getTaskServiceIcon,
  getTaskServiceResolution,
  getTaskServiceSlug,
  getTaskMessageText,
  getTaskModelLabel,
  getTaskResultPreview,
  getTaskRowView,
} from './columns'

describe('task monitor model column', () => {
  it('shows execution fields only from the immutable latest execution snapshot', () => {
    expect(getTaskModelLabel(makeTask())).toBe('')
  })

  it('shows the lightweight latest immutable execution projection after a terminal Task releases its lease', () => {
    const task = makeTask({
      latest_execution: {
        provider: 'openai',
        gateway_step: 'text',
        resolved_model: 'imagen-4',
      },
    })

    expect(getTaskModelLabel(task)).toBe('imagen-4')
  })

  it('prefers the final resolved model over the selected model', () => {
    const task = makeTask({
      selected_model: 'imagen-4',
      final_resolved_model: 'imagen-4-fallback',
      latest_execution: {
        provider: 'openai',
        gateway_step: 'text',
        resolved_model: 'imagen-4',
      },
    })

    expect(getTaskModelLabel(task)).toBe('imagen-4-fallback')
  })

  it('exposes the requested resolution for the service badge', () => {
    expect(getTaskServiceResolution(makeTask())).toBe('')
    expect(getTaskServiceResolution(makeTask({ requested_resolution: '2k' }))).toBe('2k')
  })

  it('keeps pending Tasks without an attempt in an explicit empty state', () => {
    const task = makeTask()

    expect(getTaskModelLabel(task)).toBe('')
    expect(getTaskServiceSlug(task)).toBe('')
    expect(getTaskServiceIcon('')).toBeUndefined()
    expect(getTaskServiceIcon('unknown-slug')).toBeUndefined()
  })

  it('resolves the service icon from the service slug', () => {
    expect(getTaskServiceIcon('render')).toBeDefined()
    expect(getTaskServiceIcon('variations')).toBeDefined()
    expect(getTaskServiceIcon('inpaint')).toBeDefined()
    expect(getTaskServiceIcon('cutout')).toBeDefined()
    expect(getTaskServiceIcon('fusion')).toBeDefined()
    expect(getTaskServiceIcon('upscale')).toBeDefined()
    expect(getTaskServiceIcon('outpaint')).toBeDefined()
  })

  it('strips only the matching service prefix from the message', () => {
    const withPrefix = makeTask({ prompt: 'render: draw a chair', service: 'render' })
    expect(getTaskServiceSlug(withPrefix)).toBe('render')
    expect(getTaskMessageText(withPrefix)).toBe('draw a chair')

    const otherPrefix = makeTask({ prompt: 'render: draw a chair', service: 'openai' })
    expect(getTaskMessageText(otherPrefix)).toBe('render: draw a chair')

    const noFeature = makeTask({ prompt: 'render: draw a chair' })
    expect(getTaskMessageText(noFeature)).toBe('render: draw a chair')

    const onlyPrefix = makeTask({ prompt: 'render:', service: 'render' })
    expect(getTaskMessageText(onlyPrefix)).toBe('render:')
  })

  it('truncates overlong messages with an ellipsis', () => {
    const long = makeTask({ prompt: `render: ${'x'.repeat(200)}`, service: 'render' })
    const text = getTaskMessageText(long)
    expect(text.endsWith(' ...')).toBe(true)
    expect(text.length).toBe(TASK_MESSAGE_MAX_LENGTH + 4)

    const short = makeTask({ prompt: 'short prompt', service: 'render' })
    expect(getTaskMessageText(short)).toBe('short prompt')
  })

  it('previews at most 3 result images with overflow count', () => {
    const urls = ['/uploads/a.jpg', '/uploads/b.jpg', '/uploads/c.jpg', '/uploads/d.jpg']
    const row = makeTask({
      status: 'completed',
      result: JSON.stringify({ image_urls: urls }),
    })
    const preview = getTaskResultPreview(row)
    expect(preview.urls).toEqual(urls.slice(0, 3))
    expect(preview.overflow).toBe(1)

    const single = makeTask({
      status: 'completed',
      result: JSON.stringify({ image_url: '/uploads/a.jpg' }),
    })
    expect(getTaskResultPreview(single)).toEqual({ urls: ['/uploads/a.jpg'], overflow: 0 })

    const pending = makeTask({ status: 'pending', result: JSON.stringify({ image_urls: urls }) })
    expect(getTaskResultPreview(pending)).toEqual({ urls: [], overflow: 0 })
  })

  it('memoizes the row view per row object', () => {
    const row = makeTask({ prompt: 'render: draw a chair', service: 'render' })
    const first = getTaskRowView(row)
    expect(getTaskRowView(row)).toBe(first)
    expect(first.serviceSlug).toBe('render')
    expect(first.message).toBe('draw a chair')
    expect(first.metric?.key).toBe('pending')

    const completed = makeTask({
      status: 'completed',
      prompt: 'render: draw a chair',
      service: 'render',
    })
    expect(getTaskRowView(completed).metric?.key).toBe('completed')
  })
})

function makeTask(overrides: Partial<AdminMonitorListItemModel> = {}): AdminMonitorListItemModel {
  return {
    task_id: 'task-1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'prompt',
    service: '',
    credits: 0,
    priority: 2,
    status: 'pending',
    error_msg: '',
    result: '',
    created_at: '2026-07-27T00:00:00Z',
    completed_at: null,
    ...overrides,
  } as AdminMonitorListItemModel
}
