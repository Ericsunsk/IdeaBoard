import { buildExecutionTaskRequest, EXECUTION_MODEL_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import {
  clampModelPoseStrength,
  clampModelReferenceStrength,
  type ModelNodeItem,
  type PromptNodeItem,
} from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { resolveConnectedInput, resolveSourceImageFromNode, waitTaskResult } from './flowTaskExecutorUtils'

interface FlowModelNodeContext {
  nodeId: string
  snapshotNode: ModelNodeItem
}

interface FlowModelExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
}

interface FlowModelResult {
  urls: string[]
}

export function validateFlowModelNodeInputs(
  context: FlowModelNodeContext,
  options: FlowModelExecutorOptions,
): string | null {
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')
  if (!imageInput) return 'Model 节点缺少 Image 连线'
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  if (!imageUrl) return 'Model 节点等待上游 Image 输出'

  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')
  if (!imageAInput) return 'Model 节点缺少 Model(ImageA) 连线'
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  if (!imageAUrl) return 'Model 节点等待上游 Model(ImageA) 输出'

  return null
}

export async function executeFlowModelNode(
  context: FlowModelNodeContext,
  options: FlowModelExecutorOptions,
): Promise<FlowModelResult> {
  const validationError = validateFlowModelNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')!
  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')!
  const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')
  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')

  const sourceUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  const imageBUrl = imageBInput
    ? resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
    : ''
  const promptText = promptInput?.sourceType === 'prompt'
    ? (((promptInput.snapshotNode as unknown as PromptNodeItem).text || '').trim())
    : ''
  const imageSize = await options.loadImageSizeFromUrl(sourceUrl)

  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_MODEL_META.promptPrefix} ${promptText}`,
    feature: EXECUTION_MODEL_META.feature,
    imageUrls: [sourceUrl],
    params: {
      ...buildExecutionSizeParams(imageSize),
      angle_spec: {
        yaw: node.angleSpec.yaw,
        framing: node.angleSpec.framing,
      },
      ...(imageAUrl
        ? {
            model_url: imageAUrl,
            model_strength: clampModelReferenceStrength(node.modelStrength),
          }
        : {}),
      ...(imageBUrl
        ? {
            pose_url: imageBUrl,
            pose_strength: clampModelPoseStrength(node.poseStrength),
          }
        : {}),
      background: node.background,
    },
  })

  try {
    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Model 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Model 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Model 任务执行失败'))
  }
}
