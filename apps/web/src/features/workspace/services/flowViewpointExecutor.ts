import { buildExecutionTaskRequest, EXECUTION_VIEWPOINT_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { PromptNodeItem, ViewpointNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api'
import { resolveConnectedInput, resolveSourceImageFromNode, waitTaskResult } from './flowTaskExecutorUtils'

interface FlowViewpointNodeContext {
  nodeId: string
  snapshotNode: ViewpointNodeItem
}

interface FlowViewpointExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
}

interface FlowViewpointResult {
  urls: string[]
}

export function validateFlowViewpointNodeInputs(
  context: FlowViewpointNodeContext,
  options: FlowViewpointExecutorOptions,
): string | null {
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')
  if (!imageInput) return 'Viewpoint 节点缺少 Image 连线'
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  if (!imageUrl) return 'Viewpoint 节点等待上游 Image 输出'
  if (!context.snapshotNode.selectedViewpoint) return '请选择视角'
  return null
}

export async function executeFlowViewpointNode(
  context: FlowViewpointNodeContext,
  options: FlowViewpointExecutorOptions,
): Promise<FlowViewpointResult> {
  const validationError = validateFlowViewpointNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')!
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')
  const promptText = promptInput?.sourceType === 'prompt'
    ? (((promptInput.snapshotNode as unknown as PromptNodeItem).text || '').trim())
    : ''
  const imageSize = await options.loadImageSizeFromUrl(imageUrl)

  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_VIEWPOINT_META.promptPrefix} ${promptText}`,
    feature: EXECUTION_VIEWPOINT_META.feature,
    imageUrls: [imageUrl],
    params: {
      ...buildExecutionSizeParams(imageSize),
      viewpoint: node.selectedViewpoint,
      lighting: node.lighting,
    },
  })

  try {
    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Viewpoint 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Viewpoint 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Viewpoint 任务执行失败'))
  }
}
