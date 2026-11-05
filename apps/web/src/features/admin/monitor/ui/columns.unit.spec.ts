import { describe, expect, it } from 'vitest'
import { makeAdminMonitorTask } from '@/test/factories'
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
    expect(getTaskModelLabel(makeAdminMonitorTask())).toBe('')
  })

  it('shows the lightweight latest immutable execution projection after a terminal Task releases its lease', () => {
    const task = makeAdminMonitorTask({
      latest_execution: {
        provider: 'openai',
        gateway_step: 'text',
        resolved_model: 'imagen-4',
      },
    })

    expect(getTaskModelLabel(task)).toBe('imagen-4')
  })

  it('prefers the final resolved model over the selected model', () => {
    const task = makeAdminMonitorTask({
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
    expect(getTaskServiceResolution(makeAdminMonitorTask())).toBe('')
    expect(getTaskServiceResolution(makeAdminMonitorTask({ requested_resolution: '2k' }))).toBe('2k')
  })

  it('keeps pending Tasks without an attempt in an explicit empty state', () => {
    const task = makeAdminMonitorTask()

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
    const withPrefix = makeAdminMonitorTask({ prompt: 'render: draw a chair', service: 'render' })
    expect(getTaskServiceSlug(withPrefix)).toBe('render')
    expect(getTaskMessageText(withPrefix)).toBe('draw a chair')

    const otherPrefix = makeAdminMonitorTask({ prompt: 'render: draw a chair', service: 'openai' })
    expect(getTaskMessageText(otherPrefix)).toBe('render: draw a chair')

    const noFeature = makeAdminMonitorTask({ prompt: 'render: draw a chair' })
    expect(getTaskMessageText(noFeature)).toBe('render: draw a chair')

    const onlyPrefix = makeAdminMonitorTask({ prompt: 'render:', service: 'render' })
    expect(getTaskMessageText(onlyPrefix)).toBe('render:')
  })

  it('truncates overlong messages with an ellipsis', () => {
    const long = makeAdminMonitorTask({ prompt: `render: ${'x'.repeat(200)}`, service: 'render' })
    const text = getTaskMessageText(long)
    expect(text.endsWith(' ...')).toBe(true)
    expect(text.length).toBe(TASK_MESSAGE_MAX_LENGTH + 4)

    const short = makeAdminMonitorTask({ prompt: 'short prompt', service: 'render' })
    expect(getTaskMessageText(short)).toBe('short prompt')
  })

  it('previews at most 3 result images with overflow count', () => {
    const urls = [
      '/api/v1/workspaces/ws_test123/media-assets/asset_a/content',
      '/api/v1/workspaces/ws_test123/media-assets/asset_b/content',
      '/api/v1/workspaces/ws_test123/media-assets/asset_c/content',
      '/api/v1/workspaces/ws_test123/media-assets/asset_d/content',
    ]
    const row = makeAdminMonitorTask({
      status: 'completed',
      result: JSON.stringify({ image_urls: urls }),
    })
    const preview = getTaskResultPreview(row)
    expect(preview.urls).toEqual(urls.slice(0, 3))
    expect(preview.overflow).toBe(1)

    const single = makeAdminMonitorTask({
      status: 'completed',
      result: JSON.stringify({
        image_url: '/api/v1/workspaces/ws_test123/media-assets/asset_a/content',
      }),
    })
    expect(getTaskResultPreview(single)).toEqual({
      urls: ['/api/v1/workspaces/ws_test123/media-assets/asset_a/content'],
      overflow: 0,
    })

    const pending = makeAdminMonitorTask({ status: 'pending', result: JSON.stringify({ image_urls: urls }) })
    expect(getTaskResultPreview(pending)).toEqual({ urls: [], overflow: 0 })
  })

  it('memoizes the row view per row object', () => {
    const row = makeAdminMonitorTask({ prompt: 'render: draw a chair', service: 'render' })
    const first = getTaskRowView(row)
    expect(getTaskRowView(row)).toBe(first)
    expect(first.serviceSlug).toBe('render')
    expect(first.message).toBe('draw a chair')
    expect(first.metric?.key).toBe('pending')

    const completed = makeAdminMonitorTask({
      status: 'completed',
      prompt: 'render: draw a chair',
      service: 'render',
    })
    expect(getTaskRowView(completed).metric?.key).toBe('completed')
  })
})
