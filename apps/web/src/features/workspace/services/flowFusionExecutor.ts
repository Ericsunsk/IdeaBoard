import { buildExecutionTaskRequest, EXECUTION_FUSION_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { clampImageNodeOutputCount, type FusionNodeItem } from '../types/canvasNodes'
import { computeFusionBlendWeights } from '../utils/fusionBlend'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import {
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
  waitTaskResult,
} from './flowTaskExecutorUtils'

interface FlowFusionNodeContext {
  nodeId: string
  snapshotNode: FusionNodeItem
}

interface FlowFusionExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{
    source: string
    sourceHandle?: string | null
    target: string
    targetHandle?: string | null
  }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
}

interface FlowFusionResult {
  urls: string[]
}

export function validateFlowFusionNodeInputs(
  context: FlowFusionNodeContext,
  options: FlowFusionExecutorOptions,
): string | null {
  const imageUrls = resolveConnectedImageInputs(
    options,
    context.nodeId,
    context.snapshotNode.inputImages,
  )
    .map((item) => item.url.trim())
    .filter(Boolean)
  if (imageUrls.length < 2) return 'Fusion 节点至少需要 2 张图片'
  if (imageUrls.length > 4) return 'Fusion 节点最多支持 4 张图片'

  return null
}

export async function executeFlowFusionNode(
  context: FlowFusionNodeContext,
  options: FlowFusionExecutorOptions,
): Promise<FlowFusionResult> {
  const validationError = validateFlowFusionNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageUrls = resolveConnectedImageInputs(options, context.nodeId, node.inputImages)
    .map((item) => item.url.trim())
    .filter(Boolean)
    .slice(0, 4)
  const imageWeights = computeFusionBlendWeights(imageUrls.length, node.blendPoint, node.width).slice(
    0,
    imageUrls.length,
  )
  const imageSize = await options.loadImageSizeFromUrl(imageUrls[0])

  const selfPrompt =
    typeof node.prompt === 'string' ? node.prompt.trim() : `${node.prompt ?? ''}`.trim()
  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')
  const promptText = selfPrompt || (promptInput ? resolveConnectedPromptText(options, context.nodeId) : '')
  const taskPrompt = promptText
    ? `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`
    : EXECUTION_FUSION_META.promptPrefix

  const request = buildExecutionTaskRequest({
    prompt: taskPrompt,
    feature: EXECUTION_FUSION_META.feature,
    imageUrls,
    params: {
      ...buildExecutionSizeParams(imageSize),
      image_weights: imageWeights,
    },
  })

  try {
    const outputCount = clampImageNodeOutputCount(node.outputCount)
    const resultUrls: string[] = []
    for (let index = 0; index < outputCount; index += 1) {
      const submitRes = await taskApi.submit(request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Fusion 任务失败'))
      }
      resultUrls.push(...(await waitTaskResult(submitRes.data.task_id)))
    }
    const urls = resultUrls.filter((url) => url.trim())
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls: urls.slice(0, outputCount) }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Fusion 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Fusion 任务执行失败'), { cause: error })
  }
}
