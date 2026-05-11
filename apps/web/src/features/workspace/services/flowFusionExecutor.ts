import { buildExecutionTaskRequest, EXECUTION_FUSION_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { FusionNodeItem, PromptNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { resolveConnectedInput, resolveSourceImageFromNode, waitTaskResult } from './flowTaskExecutorUtils'

interface FlowFusionNodeContext {
  nodeId: string
  snapshotNode: FusionNodeItem
}

interface FlowFusionExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
}

interface FlowFusionResult {
  urls: string[]
}

export function validateFlowFusionNodeInputs(
  context: FlowFusionNodeContext,
  options: FlowFusionExecutorOptions,
): string | null {
  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')
  if (!imageAInput) return 'Fusion 节点缺少 Image A 连线'
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  if (!imageAUrl) return 'Fusion 节点等待上游 Image A 输出'

  const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')
  if (!imageBInput) return 'Fusion 节点缺少 Image B 连线'
  const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
  if (!imageBUrl) return 'Fusion 节点等待上游 Image B 输出'

  return null
}

export async function executeFlowFusionNode(
  context: FlowFusionNodeContext,
  options: FlowFusionExecutorOptions,
): Promise<FlowFusionResult> {
  const validationError = validateFlowFusionNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')!
  const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')!
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
  const imageSize = await options.loadImageSizeFromUrl(imageAUrl)

  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')
  const promptText = promptInput?.sourceType === 'prompt'
    ? (((promptInput.snapshotNode as unknown as PromptNodeItem).text || '').trim())
    : ''

  const blendRatio = Math.max(0, Math.min(100, Math.round(Number(node.blendRatio) || 0)))
  const imageAWeight = blendRatio
  const imageBWeight = 100 - blendRatio
  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`,
    feature: EXECUTION_FUSION_META.feature,
    imageUrls: [imageAUrl, imageBUrl],
    params: {
      ...buildExecutionSizeParams(imageSize),
      blend_ratio: blendRatio,
      image_a_weight: imageAWeight,
      image_b_weight: imageBWeight,
    },
  })

  try {
    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Fusion 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Fusion 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Fusion 任务执行失败'), { cause: error })
  }
}
