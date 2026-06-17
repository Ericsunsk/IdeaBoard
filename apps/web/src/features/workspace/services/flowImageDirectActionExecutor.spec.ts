import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../shared/api/task'
import {
  resolveHdClarityStrength,
  resolveHdOutputQuality,
  submitHdUpscaleTask,
} from './flowImageDirectActionExecutor'

const mocks = vi.hoisted(() => ({
  submit: vi.fn(),
}))

vi.mock('../../../shared/api/task', () => ({
  taskApi: {
    submit: mocks.submit,
  },
}))

describe('flowImageDirectActionExecutor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.submit.mockResolvedValue({ code: 0, data: { task_id: 'task-hd' } })
  })

  it('maps HD strength to output quality without changing size semantics', () => {
    expect(resolveHdClarityStrength(undefined)).toBe(50)
    expect(resolveHdClarityStrength(75.4)).toBe(75)
    expect(resolveHdOutputQuality(0)).toBe(60)
    expect(resolveHdOutputQuality(50)).toBe(80)
    expect(resolveHdOutputQuality(100)).toBe(100)
    expect(resolveHdOutputQuality(undefined)).toBe(80)
  })

  it('keeps HD output size at the source size and maps strength to quality', async () => {
    await submitHdUpscaleTask(
      {
        url: 'https://assets.example.com/source.png',
        width: 280,
        ratio: 0.5,
        x: 0,
        y: 0,
      },
      async () => ({ width: 1200, height: 800 }),
      75,
    )

    const request = mocks.submit.mock.calls[0]?.[0] as SubmitTaskData
    expect(request.params).toMatchObject({
      original_image_url: 'https://assets.example.com/source.png',
      custom_width: 1200,
      custom_height: 800,
      original_image_width: 1200,
      original_image_height: 800,
      clarity_strength: 75,
      output_quality: 90,
    })
    expect(request.params).not.toHaveProperty('upscale_factor')
    expect(request.params).not.toHaveProperty('upscale_mode')
  })
})
