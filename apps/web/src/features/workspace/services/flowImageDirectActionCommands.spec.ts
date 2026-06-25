import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../shared/api/task'
import { EXECUTION_FEATURE } from '../config/executionContract'
import { buildFusionTaskCommand } from './flowImageDirectActionCommands'

const mocks = vi.hoisted(() => ({
  assertTaskSubmitReady: vi.fn(),
  submit: vi.fn(),
}))

vi.mock('../../../shared/api/task', () => ({
  taskApi: {
    submit: mocks.submit,
  },
}))

vi.mock('./flowTaskExecutorUtils', () => ({
  assertTaskSubmitReady: mocks.assertTaskSubmitReady,
}))

describe('flowImageDirectActionCommands', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.assertTaskSubmitReady.mockResolvedValue(undefined)
    mocks.submit.mockResolvedValue({ code: 0, data: { task_id: 'task-fusion' } })
  })

  it('checks Fusion readiness with weighted reference params', async () => {
    const command = buildFusionTaskCommand({
      prompt: '[style-fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.35, 0.65],
      sizeSource: null,
      targetSize: null,
      aspectRatio: 'auto',
    })

    await command.assertReady()

    const request = mocks.assertTaskSubmitReady.mock.calls[0]?.[0] as SubmitTaskData
    expect(request).toMatchObject({
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.35, 0.65],
        },
      },
    })
    expect(mocks.assertTaskSubmitReady.mock.calls[0]?.[1]).toBe('提交 Fusion 任务失败')
  })

  it('submits Fusion weighted reference params with size and aspect ratio', async () => {
    const command = buildFusionTaskCommand({
      prompt: '[style-fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.4, 0.6],
      sizeSource: { width: 1024, height: 768 },
      targetSize: { width: 1280, height: 720 },
      aspectRatio: '16:9',
    })

    await expect(command.submit()).resolves.toEqual({ taskId: 'task-fusion' })

    const request = mocks.submit.mock.calls[0]?.[0] as SubmitTaskData
    expect(request).toMatchObject({
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        custom_width: 1280,
        custom_height: 720,
        original_image_width: 1024,
        original_image_height: 768,
        ratio: '16:9',
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.4, 0.6],
        },
      },
    })
  })
})
