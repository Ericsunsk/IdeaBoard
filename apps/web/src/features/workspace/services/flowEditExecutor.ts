import {
  buildExecutionTaskRequest,
  EXECUTION_EDIT_MODE_META,
  EXECUTION_FACE_SWAP_META,
} from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { EditNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import {
  resolveConnectedPromptText,
  resolveConnectedInput,
  resolveSourceImageFromNode,
  waitTaskResult,
} from './flowTaskExecutorUtils'
import { EDIT_NODE_MODE } from '../types/editNodeModes'

interface FlowEditNodeContext {
  nodeId: string
  snapshotNode: EditNodeItem
}

interface FlowEditExecutorOptions {
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

interface FlowEditResult {
  urls: string[]
}

export function validateFlowEditNodeInputs(
  context: FlowEditNodeContext,
  options: FlowEditExecutorOptions,
): string | null {
  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')
  if (!imageInput) return 'Edit 节点缺少 Image 连线'
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  if (!imageUrl) return 'Edit 节点等待上游 Image 输出'

  if (node.mode === EDIT_NODE_MODE.faceSwap) {
    const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')
    if (!imageBInput) return 'Edit 节点缺少 Face(ImageB) 连线'
    const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
    if (!imageBUrl) return 'Edit 节点等待上游 Face(ImageB) 输出'
    return null
  }

  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')
  if (!promptInput || promptInput.sourceType !== 'prompt') {
    return 'Edit 节点缺少 Prompt 连线'
  }
  const promptText = resolveConnectedPromptText(options, context.nodeId)
  if (!promptText) {
    return 'Edit 节点缺少 Prompt 内容（请填写 Prompt 节点）'
  }
  return null
}

export async function executeFlowEditNode(
  context: FlowEditNodeContext,
  options: FlowEditExecutorOptions,
): Promise<FlowEditResult> {
  const validationError = validateFlowEditNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')!
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  const imageSize = await options.loadImageSizeFromUrl(imageUrl)

  try {
    if (node.mode === EDIT_NODE_MODE.faceSwap) {
      const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')!
      const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
      const request = buildExecutionTaskRequest({
        prompt: EXECUTION_FACE_SWAP_META.prompt,
        feature: EXECUTION_FACE_SWAP_META.feature,
        imageUrls: [imageUrl, imageBUrl],
        params: buildExecutionSizeParams(imageSize),
      })

      const submitRes = await taskApi.submit(request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Edit 任务失败'))
      }
      const urls = await waitTaskResult(submitRes.data.task_id)
      if (urls.length === 0) throw new Error('任务完成，但未返回图片')
      return { urls }
    }

    const promptText = resolveConnectedPromptText(options, context.nodeId)
    if (!promptText) {
      throw new Error('Edit 节点缺少 Prompt 内容（请填写 Prompt 节点）')
    }
    const modeMeta = EXECUTION_EDIT_MODE_META[node.mode]

    const request = buildExecutionTaskRequest({
      prompt: `${modeMeta.promptPrefix} ${promptText}`,
      feature: modeMeta.feature,
      imageUrls: [imageUrl],
      params: {
        original_image_url: imageUrl,
        ...buildExecutionSizeParams(imageSize),
      },
    })
    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Edit 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) throw new Error('任务完成，但未返回图片')
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Edit 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Edit 任务执行失败'), { cause: error })
  }
}
