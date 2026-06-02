import { taskApi } from '../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  type ExecutionFeature,
  buildExecutionTaskRequest,
  EXECUTION_DIRECT_ACTION_META,
  EXECUTION_PARTIAL_EDIT_META,
  EXECUTION_IMAGE_TOOL_META,
} from '../config/executionContract'
import {
  buildExecutionSizeParams,
  extendExecutionImageSize,
  scaleExecutionImageSize,
} from '../utils/executionSizeParams'
import {
  buildPromptAspectRatioTaskParams,
  type PromptAspectRatio,
} from '../config/promptAspectRatio'
import { extractTaskImageUrls } from '../utils/flowImageDirectActionTaskResult'
import { assertTaskSubmitReady, waitTaskResult } from './flowTaskExecutorUtils'
import type { ImageDirectActionSource } from '../utils/flowImageDirectActionSource'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  IMAGE_NODE_ACTION,
  type ImageDirectTaskAction,
} from '../types/imageActions'

export type ImageHdUpscaleFactor = 2 | 3 | 4

type ImageSizeLoader = (url: string) => Promise<{ width: number; height: number } | null>

type CanvasExtendPercent = {
  top: number
  right: number
  bottom: number
  left: number
}

type SubmitCanvasExtendTaskInput = {
  source: ImageDirectActionSource
  loadImageSize: ImageSizeLoader
  extendPercent: CanvasExtendPercent
  prompt?: string
}

type SubmitImageInpaintTaskInput = {
  prompt: string
  croppedImageUrl: string
  croppedMaskUrl: string
  referenceImageUrls?: string[]
  cropSize: { width: number; height: number } | null
}

const DIRECT_ACTION_FEATURES = {
  [IMAGE_NODE_ACTION.removeBg]: EXECUTION_DIRECT_ACTION_META.removeBg.feature,
  [IMAGE_NODE_ACTION.styleVariation]: EXECUTION_DIRECT_ACTION_META.styleVariation.feature,
  [IMAGE_NODE_ACTION.sketchToImage]: EXECUTION_DIRECT_ACTION_META.sketchToImage.feature,
} satisfies Record<ImageDirectTaskAction, ExecutionFeature>

export function resolveHdUpscaleFactor(strength: number | undefined): ImageHdUpscaleFactor {
  const normalized = Number.isFinite(strength)
    ? Math.max(0, Math.min(100, Math.round(strength as number)))
    : DEFAULT_IMAGE_ACTION_STRENGTH
  if (normalized >= 67) return 4
  if (normalized >= 34) return 3
  return 2
}

export function resolveSourceOutputFormat(url: string): 'jpg' | 'png' {
  try {
    const pathname = new URL(url).pathname
    return /\.png$/i.test(pathname) ? 'png' : 'jpg'
  } catch {
    const path = url.split(/[?#]/)[0] ?? ''
    return /\.png$/i.test(path) ? 'png' : 'jpg'
  }
}

export async function assertDirectActionSubmitReady(action: ImageDirectTaskAction): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: DIRECT_ACTION_FEATURES[action],
    }),
    '提交任务失败',
  )
}

export async function assertPromptImageSubmitReady(): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: EXECUTION_PARTIAL_EDIT_META.feature,
    }),
    '提交任务失败',
  )
}

export async function assertCanvasExtendSubmitReady(): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: EXECUTION_IMAGE_TOOL_META.imageExtend.feature,
    }),
    '提交任务失败',
  )
}

export async function assertHdUpscaleSubmitReady(): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: EXECUTION_IMAGE_TOOL_META.hdUpscale.feature,
    }),
    '提交任务失败',
  )
}

export async function submitPromptTextGenerationTask(
  prompt: string,
  aspectRatio: PromptAspectRatio,
): Promise<string> {
  const request = buildExecutionTaskRequest({
    prompt,
    feature: EXECUTION_DIRECT_ACTION_META.patternGen.feature,
    params: buildPromptAspectRatioTaskParams(aspectRatio),
  })
  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitDirectActionTask(
  source: ImageDirectActionSource,
  action: ImageDirectTaskAction,
  loadImageSize: ImageSizeLoader,
  strengthOverride?: number,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const strength = Number.isFinite(strengthOverride)
    ? Math.max(0, Math.min(100, Math.round(strengthOverride as number)))
    : DEFAULT_IMAGE_ACTION_STRENGTH
  const request =
    action === IMAGE_NODE_ACTION.styleVariation
      ? buildExecutionTaskRequest({
          prompt: '',
          feature: DIRECT_ACTION_FEATURES[action],
          imageUrls: [source.url],
          params: {
            original_image_url: source.url,
            ...buildExecutionSizeParams(imageSize),
            variation_strength: strength,
            strength,
          },
        })
      : buildExecutionTaskRequest({
          prompt: '',
          feature: DIRECT_ACTION_FEATURES[action],
          imageUrls: [source.url],
          params: buildExecutionSizeParams(imageSize),
        })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitPromptImageGenerationTask(
  source: ImageDirectActionSource,
  prompt: string,
  loadImageSize: ImageSizeLoader,
  aspectRatio: PromptAspectRatio,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [source.url],
    params: {
      original_image_url: source.url,
      ...buildExecutionSizeParams(imageSize),
      ...buildPromptAspectRatioTaskParams(aspectRatio),
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitImageInpaintTask(input: SubmitImageInpaintTaskInput): Promise<string> {
  const referenceImageUrls = (input.referenceImageUrls ?? [])
    .map((url) => url.trim())
    .filter(Boolean)
    .slice(0, 4)
  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${input.prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [input.croppedImageUrl],
    params: {
      original_image_url: input.croppedImageUrl,
      mask_image_url: input.croppedMaskUrl,
      reference_image_urls: referenceImageUrls,
      use_mask: true,
      ...buildExecutionSizeParams(input.cropSize),
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交局部重绘任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitCanvasExtendTask(input: SubmitCanvasExtendTaskInput): Promise<string> {
  const imageSize = await input.loadImageSize(input.source.url)
  const prompt = `${input.prompt ?? ''}`.trim()
  const request = buildExecutionTaskRequest({
    prompt: prompt ? `${EXECUTION_IMAGE_TOOL_META.imageExtend.promptPrefix} ${prompt}` : '',
    feature: EXECUTION_IMAGE_TOOL_META.imageExtend.feature,
    imageUrls: [input.source.url],
    params: {
      ...buildExecutionSizeParams(
        imageSize,
        extendExecutionImageSize(imageSize, input.extendPercent),
      ),
      extend_top: input.extendPercent.top,
      extend_right: input.extendPercent.right,
      extend_bottom: input.extendPercent.bottom,
      extend_left: input.extendPercent.left,
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitHdUpscaleTask(
  source: ImageDirectActionSource,
  loadImageSize: ImageSizeLoader,
  strength?: number,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const upscaleFactor = resolveHdUpscaleFactor(strength)
  const outputFormat = resolveSourceOutputFormat(source.url)
  const request = buildExecutionTaskRequest({
    prompt: EXECUTION_IMAGE_TOOL_META.hdUpscale.prompt,
    feature: EXECUTION_IMAGE_TOOL_META.hdUpscale.feature,
    imageUrls: [source.url],
    params: {
      original_image_url: source.url,
      ...buildExecutionSizeParams(imageSize, scaleExecutionImageSize(imageSize, upscaleFactor)),
      upscale_mode: 'enhancement',
      upscale_factor: upscaleFactor,
      output_format: outputFormat,
      output_quality: 95,
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 HD 任务失败'))
  }
  return submitRes.data.task_id
}

export async function waitExistingOrLiveTaskResult(taskId: string): Promise<string[]> {
  let task:
    | {
        status?: string
        outputs?: Array<{ role?: string; url?: string }>
        result?: string | null
        error_msg?: string
      }
    | undefined
  try {
    const taskRes = await taskApi.get(taskId)
    task = taskRes.data?.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskResult(taskId)
  }
  if (task?.status === 'completed') {
    const urls = extractTaskImageUrls(task)
    if (urls.length > 0) return urls
    throw new Error('任务完成但未返回图片')
  }
  if (task?.status === 'failed') {
    throw new Error(sanitizeApiErrorMessage(task.error_msg, '生成失败'))
  }
  return waitTaskResult(taskId)
}
