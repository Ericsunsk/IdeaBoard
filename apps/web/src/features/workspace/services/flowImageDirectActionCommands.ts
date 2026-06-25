import { taskApi, type SubmitTaskData } from '../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { buildExecutionTaskRequest, EXECUTION_FUSION_META } from '../config/executionContract'
import {
  buildPromptAspectRatioTaskParams,
  type PromptAspectRatio,
} from '../config/promptAspectRatio'
import {
  IMAGE_ACTION_MODE,
  IMAGE_ACTION_REGISTRY,
  isImageDirectTaskAction,
  type ImageTaskAction,
} from '../config/imageActionRegistry'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { ImageDirectActionSource } from '../utils/flowImageDirectActionSource'
import {
  assertCanvasExtendSubmitReady,
  assertDirectActionSubmitReady,
  assertHdUpscaleSubmitReady,
  assertPromptImageSubmitReady,
  submitCanvasExtendTask,
  submitDirectActionTask,
  submitHdUpscaleTask,
  submitImageInpaintTask,
  submitPromptImageGenerationTask,
  submitPromptTextGenerationTask,
} from './flowImageDirectActionExecutor'
import { assertTaskSubmitReady } from './flowTaskExecutorUtils'

type ImageSizeLoader = (url: string) => Promise<{ width: number; height: number } | null>

type CanvasExtendPercent = {
  top: number
  right: number
  bottom: number
  left: number
}

type ImageTaskSubmitCommand = {
  assertReady: () => Promise<void>
  submit: () => Promise<{ taskId: string }>
}

type FusionTaskImageSize = {
  width: number
  height: number
}

export function buildImageDirectTaskCommand(input: {
  source: ImageDirectActionSource
  action: ImageTaskAction
  loadImageSize: ImageSizeLoader
  strength?: number
}): ImageTaskSubmitCommand | null {
  const { source, action, loadImageSize, strength } = input
  const actionMode = IMAGE_ACTION_REGISTRY[action].mode
  if (actionMode === IMAGE_ACTION_MODE.hdUpscaleTask) {
    return {
      assertReady: () => assertHdUpscaleSubmitReady(),
      submit: async () => ({
        taskId: await submitHdUpscaleTask(source, loadImageSize, strength),
      }),
    }
  }
  if (!isImageDirectTaskAction(action)) return null
  return {
    assertReady: () => assertDirectActionSubmitReady(action),
    submit: async () => ({
      taskId: await submitDirectActionTask(source, action, loadImageSize, strength),
    }),
  }
}

export function buildCanvasExtendTaskCommand(input: {
  source: ImageDirectActionSource
  loadImageSize: ImageSizeLoader
  extendPercent: CanvasExtendPercent
  prompt?: string
}): ImageTaskSubmitCommand {
  return {
    assertReady: () => assertCanvasExtendSubmitReady(),
    submit: async () => ({
      taskId: await submitCanvasExtendTask(input),
    }),
  }
}

export function buildPromptImageTaskCommand(input: {
  source: ImageDirectActionSource
  prompt: string
  loadImageSize: ImageSizeLoader
  aspectRatio: PromptAspectRatio
}): ImageTaskSubmitCommand {
  return {
    assertReady: () => assertPromptImageSubmitReady(),
    submit: async () => ({
      taskId: await submitPromptImageGenerationTask(
        input.source,
        input.prompt,
        input.loadImageSize,
        input.aspectRatio,
      ),
    }),
  }
}

export function buildTextPromptTaskCommand(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
}): ImageTaskSubmitCommand {
  return {
    assertReady: async () => undefined,
    submit: async () => ({
      taskId: await submitPromptTextGenerationTask(input.prompt, input.aspectRatio),
    }),
  }
}

export function buildImageInpaintTaskCommand(input: {
  prompt: string
  croppedImageUrl: string
  croppedMaskUrl: string
  referenceImageUrls?: string[]
  cropSize: { width: number; height: number } | null
}): ImageTaskSubmitCommand {
  return {
    assertReady: () => assertImageInpaintTaskSubmitReady(),
    submit: async () => ({
      taskId: await submitImageInpaintTask(input),
    }),
  }
}

export async function assertImageInpaintTaskSubmitReady(): Promise<void> {
  await assertPromptImageSubmitReady()
}

type FusionTaskCommandInput = {
  prompt: string
  imageUrls: string[]
  inputWeights: number[]
  sizeSource: FusionTaskImageSize | null
  targetSize: FusionTaskImageSize | null
  aspectRatio: PromptAspectRatio
}

function buildFusionTaskRequest(input: FusionTaskCommandInput): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: input.prompt,
    feature: EXECUTION_FUSION_META.feature,
    imageUrls: input.imageUrls,
    params: {
      ...buildExecutionSizeParams(input.sizeSource, input.targetSize),
      ...buildPromptAspectRatioTaskParams(input.aspectRatio),
      fusion: {
        mode: 'weighted_reference',
        input_weights: input.inputWeights,
      },
    },
  })
}

export function buildFusionTaskCommand(input: FusionTaskCommandInput): ImageTaskSubmitCommand {
  return {
    assertReady: () => assertTaskSubmitReady(buildFusionTaskRequest(input), '提交 Fusion 任务失败'),
    submit: async () => {
      const submitRes = await taskApi.submit(buildFusionTaskRequest(input))
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Fusion 任务失败'))
      }
      return { taskId: submitRes.data.task_id }
    },
  }
}
