import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../shared/api/task'
import {
  resolveHdClarityStrength,
  submitCanvasExtendTask,
  submitDirectActionTask,
  submitHdUpscaleTask,
} from './flowImageDirectActionExecutor'
import { IMAGE_NODE_ACTION } from '../types/imageActions'

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

  it('normalizes HD clarity strength without changing size semantics', () => {
    expect(resolveHdClarityStrength(undefined)).toBe(50)
    expect(resolveHdClarityStrength(75.4)).toBe(75)
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
      custom_width: 1200,
      custom_height: 800,
      original_image_width: 1200,
      original_image_height: 800,
      hd: {
        clarity_strength: 75,
      },
    })
    expect(request.params?.hd).not.toHaveProperty('output_quality')
    expect(request.params?.hd).not.toHaveProperty('size_policy')
    expect(request.params).not.toHaveProperty('upscale_factor')
    expect(request.params).not.toHaveProperty('upscale_mode')
  })

  it('submits style variation params under the nested variation contract', async () => {
    await submitDirectActionTask(
      {
        url: 'https://assets.example.com/variation-source.png',
        width: 320,
        ratio: 1,
        x: 0,
        y: 0,
      },
      IMAGE_NODE_ACTION.styleVariation,
      async () => ({ width: 1024, height: 1024 }),
      61.6,
    )

    const request = mocks.submit.mock.calls[0]?.[0] as SubmitTaskData
    expect(request.params).toMatchObject({
      custom_width: 1024,
      custom_height: 1024,
      original_image_width: 1024,
      original_image_height: 1024,
      variation: {
        strength: 62,
      },
    })
    expect(request.params).not.toHaveProperty('variation_strength')
  })

  it('submits image extend params under the nested extend contract', async () => {
    await submitCanvasExtendTask({
      source: {
        url: 'https://assets.example.com/extend-source.png',
        width: 280,
        ratio: 0.75,
        x: 0,
        y: 0,
      },
      loadImageSize: async () => ({ width: 1000, height: 750 }),
      extendPercent: {
        top: 10,
        right: 20,
        bottom: 30,
        left: 40,
      },
      prompt: 'extend horizon',
    })

    const request = mocks.submit.mock.calls[0]?.[0] as SubmitTaskData
    expect(request.params).toMatchObject({
      custom_width: 1600,
      custom_height: 1050,
      original_image_width: 1000,
      original_image_height: 750,
      extend: {
        top_percent: 10,
        right_percent: 20,
        bottom_percent: 30,
        left_percent: 40,
      },
    })
    expect(request.params).not.toHaveProperty('extend_top')
    expect(request.params).not.toHaveProperty('extend_right')
    expect(request.params).not.toHaveProperty('extend_bottom')
    expect(request.params).not.toHaveProperty('extend_left')
  })
})
