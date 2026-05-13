import { describeWearTileMode } from '../components/nodes/config/nodeOptionConfig'
import { buildExecutionTaskRequest, EXECUTION_WEAR_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { MaskNodeItem, WearNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { cleanupFlowExecutionMaskAsset, resolveFlowExecutionMaskAsset } from './flowMaskProcessing'
import {
  parseHandlePort,
  resolveConnectedInput,
  resolveSourceImageFromNode,
  waitTaskResult,
} from './flowTaskExecutorUtils'

interface FlowWearNodeContext {
  nodeId: string
  snapshotNode: WearNodeItem
}

interface FlowWearExecutorOptions {
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

interface FlowWearResult {
  urls: string[]
}

function readWearMaskSource(maskInput: ReturnType<typeof resolveConnectedInput>): string {
  if (maskInput?.sourceType !== 'mask') return ''
  const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
  return (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
}

export function validateFlowWearNodeInputs(
  context: FlowWearNodeContext,
  options: FlowWearExecutorOptions,
): string | null {
  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')
  if (!imageAInput) return 'Wear 节点缺少 Body 连线'
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  if (!imageAUrl) return 'Wear 节点等待上游 Body 输出'

  const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')
  if (!imageBInput) return 'Wear 节点缺少 Texture 连线'
  const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
  if (!imageBUrl) return 'Wear 节点等待上游 Texture 输出'

  const hasMaskConnection = options.edges.some(
    (edge) => edge.target === context.nodeId && parseHandlePort(edge.targetHandle) === 'mask',
  )
  if (!hasMaskConnection) return null

  const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
  if (!maskInput || maskInput.sourceType !== 'mask') {
    return 'Wear 节点缺少有效 Mask 输入'
  }
  if (!readWearMaskSource(maskInput)) {
    return 'Wear 节点等待上游 Mask 输出'
  }
  return null
}

export async function executeFlowWearNode(
  context: FlowWearNodeContext,
  options: FlowWearExecutorOptions,
): Promise<FlowWearResult> {
  const validationError = validateFlowWearNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageAInput = resolveConnectedInput(options, context.nodeId, 'imageA')!
  const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')!
  const imageAUrl = resolveSourceImageFromNode(imageAInput.sourceType, imageAInput.snapshotNode)
  const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
  const imageASize = await options.loadImageSizeFromUrl(imageAUrl)
  let executionMaskAsset: Awaited<ReturnType<typeof resolveFlowExecutionMaskAsset>> | null = null

  try {
    const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
    let maskImageUrl = ''
    if (maskInput?.sourceType === 'mask') {
      executionMaskAsset = await resolveFlowExecutionMaskAsset(
        maskInput.snapshotNode as unknown as MaskNodeItem,
        { boardId: options.boardId },
      )
      maskImageUrl = executionMaskAsset.url
    }

    const tileModeLabel = describeWearTileMode(node.tileMode)
    const request = buildExecutionTaskRequest({
      prompt: `${EXECUTION_WEAR_META.promptPrefix} ${tileModeLabel}，旋转${node.rotation}°，大小${node.scale}%，融合强度${node.opacity}%`,
      feature: EXECUTION_WEAR_META.feature,
      imageUrls: [imageAUrl, imageBUrl],
      params: {
        original_image_url: imageAUrl,
        ...buildExecutionSizeParams(imageASize),
        ...(maskImageUrl
          ? {
              mask_image_url: maskImageUrl,
              use_mask: true,
            }
          : {}),
        fabric_rotation: node.rotation,
        fabric_opacity: node.opacity,
        fabric_scale: node.scale,
        tile_mode: node.tileMode,
      },
    })

    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Wear 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    return { urls }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Wear 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Wear 任务执行失败'), { cause: error })
  } finally {
    await cleanupFlowExecutionMaskAsset(executionMaskAsset, options.boardId)
  }
}
