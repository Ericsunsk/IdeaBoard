import {
  buildExecutionTaskRequest,
  EXECUTION_EDIT_MODE_META,
  EXECUTION_FACE_SWAP_META,
} from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import type { EditNodeItem, MaskNodeItem } from '../types/canvasNodes'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { taskApi } from '../../../shared/api/task'
import { uploadApi } from '../../../shared/api/upload'
import { ensureFlowMaskImageUrl } from './flowMediaUpload'
import { cleanupFlowExecutionMaskAsset, resolveFlowExecutionMaskAsset } from './flowMaskProcessing'
import {
  cleanupFlowLocalInpaintAssets,
  composeFlowLocalInpaint,
  prepareFlowLocalInpaint,
} from './flowLocalInpaint'
import {
  parseHandlePort,
  resolveConnectedPromptText,
  resolveConnectedInput,
  resolveSourceImageFromNode,
  waitTaskResult,
} from './flowTaskExecutorUtils'

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

async function buildMaskedImageDataUrl(
  imageUrl: string,
  maskDataUrl: string,
): Promise<string | null> {
  let imageBitmap: ImageBitmap | null = null
  let maskBitmap: ImageBitmap | null = null
  try {
    const [imageResponse, maskResponse] = await Promise.all([fetch(imageUrl), fetch(maskDataUrl)])
    const [imageBlob, maskBlob] = await Promise.all([imageResponse.blob(), maskResponse.blob()])
    imageBitmap = await createImageBitmap(imageBlob)
    maskBitmap = await createImageBitmap(maskBlob)

    const width = imageBitmap.width
    const height = imageBitmap.height
    if (!width || !height) return null

    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    ctx.drawImage(imageBitmap, 0, 0, width, height)

    const maskCanvas = document.createElement('canvas')
    maskCanvas.width = width
    maskCanvas.height = height
    const maskCtx = maskCanvas.getContext('2d')
    if (!maskCtx) return null
    maskCtx.drawImage(maskBitmap, 0, 0, width, height)

    const maskImage = maskCtx.getImageData(0, 0, width, height)
    const { data } = maskImage
    for (let i = 0; i < data.length; i += 4) {
      const alpha = Math.min(255, data[i + 3] * 2)
      data[i] = 255
      data[i + 1] = 255
      data[i + 2] = 255
      data[i + 3] = alpha
    }
    maskCtx.putImageData(maskImage, 0, 0)

    ctx.globalCompositeOperation = 'destination-in'
    ctx.drawImage(maskCanvas, 0, 0, width, height)
    ctx.globalCompositeOperation = 'source-over'
    return canvas.toDataURL('image/png')
  } catch {
    return null
  } finally {
    imageBitmap?.close()
    maskBitmap?.close()
  }
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

  const hasMaskConnection = options.edges.some(
    (edge) => edge.target === context.nodeId && parseHandlePort(edge.targetHandle) === 'mask',
  )
  const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
  if (hasMaskConnection) {
    if (!maskInput || maskInput.sourceType !== 'mask') return 'Edit 节点缺少有效 Mask 输入'
    const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
    const rawMaskImageUrl = (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
    if (!rawMaskImageUrl) return 'Edit 节点等待上游 Mask 输出'
  }

  if (node.mode === 'face-swap') {
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
    if (node.mode === 'face-swap') {
      const imageBInput = resolveConnectedInput(options, context.nodeId, 'imageB')!
      const imageBUrl = resolveSourceImageFromNode(imageBInput.sourceType, imageBInput.snapshotNode)
      const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
      let executionMaskAsset: Awaited<ReturnType<typeof resolveFlowExecutionMaskAsset>> | null =
        null
      let temporaryFaceSourceUrl = ''

      let faceSourceUrl = imageBUrl
      try {
        if (maskInput?.sourceType === 'mask') {
          const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
          executionMaskAsset = await resolveFlowExecutionMaskAsset(maskNode, {
            boardId: options.boardId,
          })
          if (!executionMaskAsset.url) {
            throw new Error('Edit 节点等待上游 Mask 输出')
          }
          const maskedFaceDataUrl = await buildMaskedImageDataUrl(imageBUrl, executionMaskAsset.url)
          if (!maskedFaceDataUrl) {
            throw new Error('无法生成分割人脸，请重试')
          }
          faceSourceUrl = await ensureFlowMaskImageUrl(maskedFaceDataUrl, {
            boardId: options.boardId,
          })
          temporaryFaceSourceUrl = faceSourceUrl
        }

        const request = buildExecutionTaskRequest({
          prompt: EXECUTION_FACE_SWAP_META.prompt,
          feature: EXECUTION_FACE_SWAP_META.feature,
          imageUrls: [imageUrl, faceSourceUrl],
          params: buildExecutionSizeParams(imageSize),
        })

        const submitRes = await taskApi.submit(request)
        if (submitRes.code !== 0 || !submitRes.data?.task_id) {
          throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Edit 任务失败'))
        }
        const urls = await waitTaskResult(submitRes.data.task_id)
        if (urls.length === 0) throw new Error('任务完成，但未返回图片')
        return { urls }
      } finally {
        await cleanupFlowExecutionMaskAsset(executionMaskAsset, options.boardId)
        const normalizedBoardId = `${options.boardId ?? ''}`.trim()
        if (temporaryFaceSourceUrl && normalizedBoardId) {
          await Promise.allSettled([uploadApi.recycle(temporaryFaceSourceUrl, normalizedBoardId)])
        }
      }
    }

    const promptText = resolveConnectedPromptText(options, context.nodeId)
    if (!promptText) {
      throw new Error('Edit 节点缺少 Prompt 内容（请填写 Prompt 节点）')
    }
    const modeMeta = EXECUTION_EDIT_MODE_META[node.mode]
    let localInpaint: Awaited<ReturnType<typeof prepareFlowLocalInpaint>> | null = null
    let executionMaskAsset: Awaited<ReturnType<typeof resolveFlowExecutionMaskAsset>> | null = null
    try {
      const maskInput = resolveConnectedInput(options, context.nodeId, 'mask')
      let maskImageUrl = ''
      if (maskInput?.sourceType === 'mask') {
        const maskNode = maskInput.snapshotNode as unknown as MaskNodeItem
        executionMaskAsset = await resolveFlowExecutionMaskAsset(maskNode, {
          boardId: options.boardId,
        })
        maskImageUrl = executionMaskAsset.url
        if (!maskImageUrl) {
          throw new Error('Edit 节点等待上游 Mask 输出')
        }
        if (node.mode === 'partial-edit' || node.mode === 'bg-swap') {
          localInpaint = await prepareFlowLocalInpaint({
            originalImageUrl: imageUrl,
            maskUrl: maskImageUrl,
            boardId: options.boardId,
          })
        }
      }

      const requestImageUrl = localInpaint?.croppedImageUrl || imageUrl
      const requestMaskImageUrl = localInpaint?.croppedMaskUrl || maskImageUrl
      const requestSize = localInpaint?.cropSize || imageSize

      const request = buildExecutionTaskRequest({
        prompt: `${modeMeta.promptPrefix} ${promptText}`,
        feature: modeMeta.feature,
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
        },
      })
      const submitRes = await taskApi.submit(request)
      if (submitRes.code !== 0 || !submitRes.data?.task_id) {
        throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Edit 任务失败'))
      }
      const urls = await waitTaskResult(submitRes.data.task_id)
      if (urls.length === 0) throw new Error('任务完成，但未返回图片')
      if (!localInpaint) return { urls }
      const composedUrl = await composeFlowLocalInpaint({
        originalImageUrl: imageUrl,
        croppedResultUrl: urls[0],
        croppedMaskUrl: localInpaint.croppedMaskUrl,
        bbox: localInpaint.bbox,
        boardId: options.boardId,
      })
      return { urls: [composedUrl] }
    } finally {
      await cleanupFlowExecutionMaskAsset(executionMaskAsset, options.boardId)
      await cleanupFlowLocalInpaintAssets(localInpaint, options.boardId)
    }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Edit 任务执行失败'
    throw new Error(sanitizeApiErrorMessage(msg, 'Edit 任务执行失败'), { cause: error })
  }
}
