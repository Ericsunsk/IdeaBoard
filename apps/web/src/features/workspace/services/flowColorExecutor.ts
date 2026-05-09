import { buildExecutionTaskRequest, EXECUTION_DIRECTED_RECOLOR_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import {
  DEFAULT_COLOR_STRENGTH,
  isColorSingleColorMode,
  normalizeColorNodeColors,
  type CanvasConnectionNodeType,
  type ColorNodeItem,
  type MaskNodeItem,
  type PromptNodeItem,
} from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { cleanupFlowExecutionMaskAsset, resolveFlowExecutionMaskAsset } from './flowMaskProcessing'
import { cleanupFlowLocalInpaintAssets, composeFlowLocalInpaint, prepareFlowLocalInpaint } from './flowLocalInpaint'
import { parseHandlePort, resolveConnectedExecutionInput, resolveConnectedInput, resolveSourceImageFromNode, waitTaskResult } from './flowTaskExecutorUtils'

interface FlowColorNodeContext {
  nodeId: string
  snapshotNode: ColorNodeItem
}

interface FlowColorExecutorOptions {
  nodes: Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
  edges: Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
  boardId?: string
}

interface FlowColorResult {
  urls: string[]
}

function resolveFlowColorTargetColor(node: ColorNodeItem): string {
  const normalizedColors = normalizeColorNodeColors(Array.isArray(node.colors) ? node.colors : [])
  return normalizedColors[0] || ''
}

function resolveFlowColorStrength(node: ColorNodeItem): number {
  const raw = Number(node.strength)
  const next = Number.isFinite(raw) ? Math.round(raw) : DEFAULT_COLOR_STRENGTH
  return Math.max(0, Math.min(100, next))
}

function resolveMaskSourceImageUrl(maskNode: MaskNodeItem): string {
  return (maskNode.sourceImageUrl || '').trim()
}

function resolveConnectedMaskImageSource(
  options: FlowColorExecutorOptions,
  maskNodeId: string,
): { sourceType: CanvasConnectionNodeType; snapshotNode: Record<string, unknown> } | null {
  return resolveConnectedExecutionInput(options, maskNodeId, 'image')
}

function resolveEffectiveMaskSourceImageUrl(options: FlowColorExecutorOptions, maskNodeId: string, maskNode: MaskNodeItem): string {
  const connected = resolveConnectedMaskImageSource(options, maskNodeId)
  if (connected) {
    const url = resolveSourceImageFromNode(connected.sourceType, connected.snapshotNode).trim()
    if (url) return url
  }
  return resolveMaskSourceImageUrl(maskNode)
}

function resolveConnectedMaskNodeId(options: FlowColorExecutorOptions, colorNodeId: string): string {
  const maskEdge = options.edges.find(
    edge => edge.target === colorNodeId && parseHandlePort(edge.targetHandle) === 'mask',
  )
  return maskEdge?.source || ''
}

export function validateFlowColorNodeInputs(
  context: FlowColorNodeContext,
  options: FlowColorExecutorOptions,
): string | null {
  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')
  if (!imageInput) {
    return 'Color 节点缺少 Image 连线'
  }
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  if (!imageUrl) {
    return 'Color 节点等待上游 Image 输出'
  }

  const hasMaskConnection = options.edges.some(
    edge => edge.target === context.nodeId && parseHandlePort(edge.targetHandle) === 'mask',
  )
  if (hasMaskConnection) {
    const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
    if (!maskInput || maskInput.sourceType !== 'mask') {
      return 'Color 节点缺少有效 Mask 输入'
    }
    const maskNodeId = resolveConnectedMaskNodeId(options, context.nodeId)
    const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
    const maskSourceImageUrl = maskNodeId
      ? resolveEffectiveMaskSourceImageUrl(options, maskNodeId, maskNode)
      : resolveMaskSourceImageUrl(maskNode)
    if (imageUrl && maskSourceImageUrl && imageUrl !== maskSourceImageUrl) {
      return 'Color 节点的 Image 与 Mask 来源不一致，请连接同一张图片或只连接 Mask'
    }
    const rawMaskImageUrl = (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
    if (!rawMaskImageUrl) {
      return 'Color 节点等待上游 Mask 输出'
    }
  }

  const targetColor = resolveFlowColorTargetColor(node)
  if (!isColorSingleColorMode(node) || !targetColor) {
    return 'Color 节点缺少目标颜色'
  }
  return null
}

export async function executeFlowColorNode(
  context: FlowColorNodeContext,
  options: FlowColorExecutorOptions,
): Promise<FlowColorResult> {
  const validationError = validateFlowColorNodeInputs(context, options)
  if (validationError) throw new Error(validationError)

  const node = context.snapshotNode
  const imageInput = resolveConnectedInput(options, context.nodeId, 'image')!
  const imageUrl = resolveSourceImageFromNode(imageInput.sourceType, imageInput.snapshotNode)
  const promptInput = resolveConnectedInput(options, context.nodeId, 'prompt')
  const promptText = promptInput?.sourceType === 'prompt'
    ? (((promptInput.snapshotNode as unknown as PromptNodeItem).text || '').trim())
    : ''

  const targetColor = resolveFlowColorTargetColor(node)
  const strength = resolveFlowColorStrength(node)
  const prompt = `${EXECUTION_DIRECTED_RECOLOR_META.promptPrefix} ${promptText ? `${promptText}，` : ''}目标颜色：${targetColor}，强度：${strength}`
  let localInpaint: Awaited<ReturnType<typeof prepareFlowLocalInpaint>> | null = null
  let executionMaskAsset: Awaited<ReturnType<typeof resolveFlowExecutionMaskAsset>> | null = null

  try {
    const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
    let maskImageUrl = ''
    if (maskInput?.sourceType === 'mask') {
      const maskNodeId = resolveConnectedMaskNodeId(options, context.nodeId)
      const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
      const maskSourceImageUrl = maskNodeId
        ? resolveEffectiveMaskSourceImageUrl(options, maskNodeId, maskNode)
        : resolveMaskSourceImageUrl(maskNode)
      if (imageUrl && maskSourceImageUrl && imageUrl !== maskSourceImageUrl) {
        throw new Error('Color 节点的 Image 与 Mask 来源不一致，请连接同一张图片或只连接 Mask')
      }
      const rawMaskImageUrl = (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
      if (rawMaskImageUrl) {
        executionMaskAsset = await resolveFlowExecutionMaskAsset(maskNode, { boardId: options.boardId })
        maskImageUrl = executionMaskAsset.url
        localInpaint = await prepareFlowLocalInpaint({
          originalImageUrl: imageUrl,
          maskUrl: maskImageUrl,
          boardId: options.boardId,
        })
      }
    }

    const imageSize = await options.loadImageSizeFromUrl(imageUrl)
    const requestImageUrl = localInpaint?.croppedImageUrl || imageUrl
    const requestMaskImageUrl = localInpaint?.croppedMaskUrl || maskImageUrl
    const requestSize = localInpaint?.cropSize || imageSize
    const request = buildExecutionTaskRequest({
      prompt,
      feature: EXECUTION_DIRECTED_RECOLOR_META.feature,
      imageUrls: [requestImageUrl],
      params: {
        original_image_url: requestImageUrl,
        ...buildExecutionSizeParams(requestSize),
        ...(requestMaskImageUrl
          ? {
              mask_image_url: requestMaskImageUrl,
              use_mask: true,
            }
          : {}),
        target_color: targetColor,
        variation_strength: strength,
        strength,
      },
    })

    const submitRes = await taskApi.submit(request)
    if (submitRes.code !== 0 || !submitRes.data?.task_id) {
      throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Color 任务失败'))
    }
    const urls = await waitTaskResult(submitRes.data.task_id)
    if (urls.length === 0) {
      throw new Error('任务完成，但未返回图片')
    }
    if (!localInpaint) return { urls }
    const composedUrl = await composeFlowLocalInpaint({
      originalImageUrl: imageUrl,
      croppedResultUrl: urls[0],
      croppedMaskUrl: localInpaint.croppedMaskUrl,
      bbox: localInpaint.bbox,
      boardId: options.boardId,
    })
    return { urls: [composedUrl] }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Color 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Color 任务执行失败'))
  } finally {
    await cleanupFlowExecutionMaskAsset(executionMaskAsset, options.boardId)
    await cleanupFlowLocalInpaintAssets(localInpaint, options.boardId)
  }
}
