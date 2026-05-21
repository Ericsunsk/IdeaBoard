import { buildExecutionTaskRequest, EXECUTION_ENHANCE_META } from '../config/executionContract'
import {
  buildExecutionSizeParams,
  scaleExecutionImageSize,
} from '../utils/executionSizeParams'
import type { EnhanceNodeItem, MaskNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { cleanupFlowExecutionMaskAsset, resolveFlowExecutionMaskAsset } from './flowMaskProcessing'
import {
  resolveConnectedInput,
  resolveSourceImageFromNode,
  waitTaskResult,
} from './flowTaskExecutorUtils'

interface FlowEnhanceNodeContext {
  nodeId: string
  snapshotNode: EnhanceNodeItem
}

interface FlowEnhanceExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{
    source: string
    sourceHandle?: string | null
    target: string
    targetHandle?: string | null
  }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
  boardId?: string
}

interface FlowEnhanceResult {
  urls: string[]
}

export function validateFlowEnhanceNodeInputs(
  context: FlowEnhanceNodeContext,
  options: FlowEnhanceExecutorOptions,
): string | null {
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')
  if (!imageInput) {
    return 'Enhance 节点缺少 Image 连线'
  }
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  if (!imageUrl) {
    return 'Enhance 节点等待上游 Image 输出'
  }
  const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
  if (maskInput?.sourceType === 'mask') {
    const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
    const maskImageUrl = (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
    if (!maskImageUrl) {
      return 'Enhance 节点等待上游 Mask 输出'
    }
  }
  return null
}

export async function executeFlowEnhanceNode(
  context: FlowEnhanceNodeContext,
  options: FlowEnhanceExecutorOptions,
): Promise<FlowEnhanceResult> {
  const validationError = validateFlowEnhanceNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')!
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  let executionMaskAsset: Awaited<ReturnType<typeof resolveFlowExecutionMaskAsset>> | null = null

  try {
    let maskImageUrl = ''
    const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
    if (maskInput?.sourceType === 'mask') {
      const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
      executionMaskAsset = await resolveFlowExecutionMaskAsset(maskNode, {
        boardId: options.boardId,
      })
      maskImageUrl = executionMaskAsset.url
      if (!maskImageUrl) {
        throw new Error('Enhance 节点等待上游 Mask 输出')
      }
    }

    const imageSize = await options.loadImageSizeFromUrl(imageUrl)

    const taskId = await (async () => {
      const request = buildExecutionTaskRequest({
        prompt: EXECUTION_ENHANCE_META.hdUpscale.prompt,
        feature: EXECUTION_ENHANCE_META.hdUpscale.feature,
        imageUrls: [imageUrl],
        params: {
          original_image_url: imageUrl,
          ...buildExecutionSizeParams(
            imageSize,
            maskImageUrl ? imageSize : scaleExecutionImageSize(imageSize, node.upscaleFactor),
          ),
          ...(maskImageUrl ? { mask_image_url: maskImageUrl, use_mask: true } : {}),
          upscale_mode: node.upscaleMode,
          upscale_factor: node.upscaleFactor,
          output_format: node.outputFormat,
          output_quality: node.outputQuality,
        },
      })
      const submitRes = await taskApi.submit(request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Enhance 任务失败'))
      }
      return submitRes.data.task_id
    })()

    const urls = await waitTaskResult(taskId)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Enhance 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Enhance 任务执行失败'), { cause: error })
  } finally {
    await cleanupFlowExecutionMaskAsset(executionMaskAsset, options.boardId)
  }
}
