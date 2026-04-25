import type { SubmitTaskData } from '../../../shared/api/index'
import { describeModelAngleSpec, describeWearTileMode } from '../components/nodes/nodeOptionConfig'
import {
  buildExecutionTaskRequest,
  EXECUTION_COLOR_SERIES_META,
  EXECUTION_ENHANCE_META,
  EXECUTION_DIRECTED_RECOLOR_META,
  EXECUTION_EDIT_MODE_META,
  EXECUTION_FACE_SWAP_META,
  EXECUTION_FUSION_META,
  EXECUTION_MODEL_META,
  EXECUTION_VIEWPOINT_META,
  EXECUTION_WEAR_META,
} from '../config/executionContract'
import {
  enhanceModeSupportsPrompt,
  getEditNodePortLabel,
} from '../config/canvasNodeConnectionContract'
import {
  clampModelPoseStrength,
  clampModelReferenceStrength,
  DEFAULT_COLOR_STRENGTH,
  isColorSingleColorMode,
  normalizeColorNodeColors,
} from '../types/canvasNodes'
import type { NodeConnectionStatus } from './useCanvasNodeConnections'
import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeType,
  CanvasImageItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

interface MessageApiLike {
  success(content: string): unknown
  warning(content: string): unknown
  error(content: string): unknown
}

interface ApiResponseLike<TData = Record<string, unknown>> {
  code?: number
  msg?: string
  data?: TData
  result_url?: string
}

interface SubmitTaskOptionsLike {
  request: SubmitTaskData
  source?: CanvasImageItem
  outputNodeId?: number | null
  successMessage: string
  failureMessage?: string
  onCompleted?: (urls: string[]) => void
  onFailed?: () => void
}

interface StandardSubmitBuildOptions {
  nodeType: CanvasExecutionOutputNodeType
  nodeId: number
  request: SubmitTaskData
  sourceUrl: string
  placementX: number
  placementY: number
  successMessage: string
  failureMessage: string
}

type PlacedImageInput = { url: string; placementX: number; placementY: number }
type PlacedMaskInput = {
  dataUrl: string
  placementX: number
  placementY: number
  sourceImageUrl: string
  sourceImageRatio: number
  sourceConnected: boolean
}

type StandardEditMode = Exclude<EditNodeItem['mode'], 'face-swap'>

interface UseNodeExecutorsOptions {
  message: MessageApiLike
  messages: import('vue').Ref<CanvasImageItem[]>
  selectedIds: import('vue').Ref<number[]>
  imageNodes: import('vue').Ref<ImageNodeItem[]>
  maskNodes: import('vue').Ref<MaskNodeItem[]>
  activeImageNodeId: import('vue').Ref<number | null>
  activeMaskNodeId: import('vue').Ref<number | null>
  getViewpointLabel: (key: string) => string
  getViewpointLightingLabel: (key: ViewpointNodeItem['lighting']) => string
  getNodeConnectionStatus: (
    nodeType: CanvasConnectionNodeType,
    node: { id: number },
  ) => NodeConnectionStatus
  triggerNodeConnectionError: (nodeType: CanvasConnectionNodeType, nodeId: number, ports: string[]) => void
  resolveNodePromptText: (nodeType: CanvasConnectionNodeType, nodeId: number, fallback?: string) => string
  getConnectedOutputImageTarget: (
    fromNodeType: CanvasConnectionNodeType,
    fromNodeId: number,
    fromPort?: 'image',
  ) => ImageNodeItem | null
  resolvePreferredOutputNodeId: (connectedOutputNodeId: number | null) => number | null
  buildPlacementSource: (url: string, x: number, y: number) => CanvasImageItem
  getMissingRequiredNodeInputs: (
    nodeType: CanvasConnectionNodeType,
    node: { id: number },
    resolvedInputs: Partial<Record<CanvasConnectionPort, unknown>>,
  ) => CanvasConnectionPort[]
  warnMissingNodeInputs: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    missingPorts: string[],
    formatMessage: (state: { disconnectedPorts: string[]; pendingPorts: string[] }) => string,
  ) => boolean
  submitTaskAndPoll: (options: SubmitTaskOptionsLike) => Promise<string | null | void>
  appendGeneratedImages: (urls: string[], source?: CanvasImageItem, outputNodeId?: number | null) => void
  appendWorkflowNodeOutputs: (
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    urls: string[],
  ) => void
  normalizeHexColor: (raw: string) => string
  getImageNodePreferredImage: (node: ImageNodeItem | null) => { url: string; ratio: number } | null
  resolveConnectedPlacedImageInput: (
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'image' | 'imageA' | 'imageB',
  ) => PlacedImageInput | null
  resolveConnectedPlacedMaskInput: (
    toNodeType: CanvasConnectionNodeType,
    toNodeId: number,
    toPort: 'mask',
  ) => Promise<PlacedMaskInput | null>
  buildFullMaskDataUrl: (imageUrl: string) => Promise<string | null>
  refineExecutionMaskDataUrl: (maskDataUrl: string, refineAmount: number, invertMask: boolean) => Promise<string>
  dataUrlToFile: (dataUrl: string, fileName: string) => Promise<File>
  uploadImage: (file: File) => Promise<ApiResponseLike<{ url?: string }>>
  setExecutionNodeRunning: (
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    running: boolean,
  ) => void
  setExecutionNodeLastRunFailed: (
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    failed: boolean,
  ) => void
  isExecutionNodeRunning?: (
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
  ) => boolean
}

export function useNodeExecutors(options: UseNodeExecutorsOptions) {
  function getEditModeMeta(mode: StandardEditMode) {
    return EXECUTION_EDIT_MODE_META[mode]
  }

  function resolveFailureMessage(error: unknown, fallback: string): string {
    const maybeError = error as { msg?: string; message?: string } | null
    return maybeError?.msg || maybeError?.message || fallback
  }

  function markExecutionRunning(nodeType: CanvasExecutionOutputNodeType, nodeId: number) {
    options.setExecutionNodeRunning(nodeType, nodeId, true)
    options.setExecutionNodeLastRunFailed(nodeType, nodeId, false)
  }

  function markExecutionSucceeded(nodeType: CanvasExecutionOutputNodeType, nodeId: number) {
    options.setExecutionNodeRunning(nodeType, nodeId, false)
    options.setExecutionNodeLastRunFailed(nodeType, nodeId, false)
  }

  function markExecutionFailed(nodeType: CanvasExecutionOutputNodeType, nodeId: number) {
    options.setExecutionNodeRunning(nodeType, nodeId, false)
    options.setExecutionNodeLastRunFailed(nodeType, nodeId, true)
  }

  function beginExecution(nodeType: CanvasExecutionOutputNodeType, nodeId: number): boolean {
    if (options.isExecutionNodeRunning?.(nodeType, nodeId)) {
      return false
    }
    markExecutionRunning(nodeType, nodeId)
    return true
  }

  function failExecution(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    message: string,
  ) {
    options.message.error(message)
    markExecutionFailed(nodeType, nodeId)
  }

  function failExecutionFromError(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    error: unknown,
    fallback: string,
  ) {
    failExecution(nodeType, nodeId, resolveFailureMessage(error, fallback))
  }

  function formatStrictMissingInputMessage(
    nodeLabel: string,
    disconnectedPorts: string[],
    pendingPorts: string[],
    formatLabel: (port: string) => string,
    promptContentMissing = false,
  ) {
    const missingLabels = [
      ...disconnectedPorts.map(formatLabel),
      ...(promptContentMissing ? ['Prompt 内容（请填写 Prompt 节点）'] : []),
    ]
    const pendingLabels = pendingPorts.map(formatLabel)

    if (pendingLabels.length > 0 && missingLabels.length === 0) {
      return `${nodeLabel} 节点等待上游 ${pendingLabels.join(' / ')} 输出`
    }
    if (pendingLabels.length > 0) {
      return `${nodeLabel} 节点缺少 ${missingLabels.join(' / ')}，且等待上游 ${pendingLabels.join(' / ')} 输出`
    }
    if (missingLabels.length > 0) {
      return `${nodeLabel} 节点缺少 ${missingLabels.join(' / ')}`
    }
    return `${nodeLabel} 节点缺少必填输入`
  }

  function cacheNodeOutputs(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    urls: string[],
  ) {
    options.appendWorkflowNodeOutputs(nodeType, nodeId, urls)
  }

  function resolveExecutionOutputNodeId(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
  ): number | null {
    const connectedOutputNodeId = options.getConnectedOutputImageTarget(nodeType, nodeId)?.id ?? null
    return options.resolvePreferredOutputNodeId(connectedOutputNodeId)
  }

  function buildStandardSubmitOptions({
    nodeType,
    nodeId,
    request,
    sourceUrl,
    placementX,
    placementY,
    successMessage,
    failureMessage,
  }: StandardSubmitBuildOptions): SubmitTaskOptionsLike {
    return {
      request,
      source: options.buildPlacementSource(sourceUrl, placementX, placementY),
      outputNodeId: resolveExecutionOutputNodeId(nodeType, nodeId),
      successMessage,
      failureMessage,
      onCompleted: urls => cacheNodeOutputs(nodeType, nodeId, urls),
    }
  }

  async function submitExecutionTask(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    submitOptions: SubmitTaskOptionsLike,
  ) {
    markExecutionRunning(nodeType, nodeId)
    return options.submitTaskAndPoll({
      ...submitOptions,
      onCompleted: (urls) => {
        markExecutionSucceeded(nodeType, nodeId)
        submitOptions.onCompleted?.(urls)
      },
      onFailed: () => {
        markExecutionFailed(nodeType, nodeId)
        submitOptions.onFailed?.()
      },
    })
  }

  function resolveConnectedModelPoseImageInput(node: ModelNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('model', node.id, 'imageB')
  }

  function resolveConnectedModelImageInput(node: ModelNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('model', node.id, 'image')
  }

  function resolveConnectedModelReferenceImageInput(node: ModelNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('model', node.id, 'imageA')
  }

  function resolveConnectedViewpointImageInput(node: ViewpointNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('viewpoint', node.id, 'image')
  }

  function resolveConnectedViewpointPromptInput(node: ViewpointNodeItem): string {
    return options.resolveNodePromptText('viewpoint', node.id, '')
  }

  function resolveConnectedEditImageInput(node: EditNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('edit', node.id, 'image')
  }

  function resolveConnectedEditSourceImageInput(node: EditNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('edit', node.id, 'imageB')
  }

  async function resolveConnectedEditMaskInput(node: EditNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('edit', node.id, 'mask')
  }

  function resolveConnectedEditPromptInput(node: EditNodeItem): string {
    return options.resolveNodePromptText('edit', node.id, '')
  }

  function resolveConnectedFusionImageInput(node: FusionNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('fusion', node.id, port)
  }

  function resolveConnectedFusionPromptInput(node: FusionNodeItem): string {
    return options.resolveNodePromptText('fusion', node.id, '')
  }

  function resolveConnectedEnhanceImageInput(node: EnhanceNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('enhance', node.id, 'image')
  }

  async function resolveConnectedEnhanceMaskInput(node: EnhanceNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('enhance', node.id, 'mask')
  }

  function resolveConnectedEnhancePromptInput(node: EnhanceNodeItem): string {
    return options.resolveNodePromptText('enhance', node.id, '')
  }

  function resolveConnectedWearImageInput(node: WearNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('wear', node.id, port)
  }

  async function resolveConnectedWearMaskInput(node: WearNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('wear', node.id, 'mask')
  }

  async function resolveColorGenerationContext(
    nodeType: 'color',
    node: { id: number; x: number; y: number },
  ): Promise<{
    originalImageUrl: string
    maskDataUrl: string
    placementX: number
    placementY: number
    promptText: string
  } | null> {
    const connectedImage = options.resolveConnectedPlacedImageInput(nodeType, node.id, 'image')
    const connectedMask = await options.resolveConnectedPlacedMaskInput(nodeType, node.id, 'mask')
    const promptText = options.resolveNodePromptText(nodeType, node.id, '')
    const connectionStatus = options.getNodeConnectionStatus(nodeType, node)

    if (connectionStatus.incoming.mask && !connectedMask) {
      options.triggerNodeConnectionError(nodeType, node.id, ['mask'])
      options.message.warning(
        connectionStatus.pendingIncoming.mask
          ? 'Color 节点等待上游 Mask 输出'
          : 'Color 节点蒙版不可用，请检查 Mask 节点是否存在有效源图',
      )
      return null
    }

    const missingPorts = options.getMissingRequiredNodeInputs(nodeType, node, {
      image: connectedImage,
    })
    if (options.warnMissingNodeInputs(nodeType, node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'image'
          ? 'Image'
          : port === 'mask'
            ? 'Mask'
            : port
      return formatStrictMissingInputMessage('Color', disconnectedPorts, pendingPorts, formatLabel)
    })) return null

    if (connectedMask && connectedImage!.url !== connectedMask.sourceImageUrl) {
      options.triggerNodeConnectionError(nodeType, node.id, ['image', 'mask'])
      options.message.warning('Color 节点的 Image 与 Mask 来源不一致，请连接同一张图片或只连接 Mask')
      return null
    }

    return {
      originalImageUrl: connectedImage!.url,
      maskDataUrl: connectedMask?.dataUrl || '',
      placementX: connectedImage!.placementX,
      placementY: connectedImage!.placementY,
      promptText,
    }
  }

  async function handleDirectedRecolorGenerate(
    nodeType: 'color',
    node: { id: number; x: number; y: number },
    targetColor: string,
    strength: number,
  ) {
    const resolvedContext = await resolveColorGenerationContext(nodeType, node)
    if (!resolvedContext) return

    if (!beginExecution(nodeType, node.id)) return
    const connectedOutputNodeId = options.getConnectedOutputImageTarget(nodeType, node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    try {
      const maskFile = await options.dataUrlToFile(resolvedContext.maskDataUrl, `mask-${Date.now()}.png`)
      const uploadRes = await options.uploadImage(maskFile)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) {
        failExecution(nodeType, node.id, uploadRes.msg || '蒙版上传失败')
        return
      }

      const placementSource = options.buildPlacementSource(
        resolvedContext.originalImageUrl,
        resolvedContext.placementX,
        resolvedContext.placementY,
      )
      let maskImageUrl = ''
      if (resolvedContext.maskDataUrl) {
        const maskFile = await options.dataUrlToFile(resolvedContext.maskDataUrl, `mask-${Date.now()}.png`)
        const uploadRes = await options.uploadImage(maskFile)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          failExecution(nodeType, node.id, uploadRes.msg || '蒙版上传失败')
          return
        }
        maskImageUrl = uploadRes.data.url
      }

      await submitExecutionTask(nodeType, node.id, {
        request: buildExecutionTaskRequest({
          prompt: `${EXECUTION_DIRECTED_RECOLOR_META.promptPrefix} ${resolvedContext.promptText ? `${resolvedContext.promptText}，` : ''}目标颜色：${targetColor}，强度：${strength}`,
          feature: EXECUTION_DIRECTED_RECOLOR_META.feature,
          imageUrls: [resolvedContext.originalImageUrl],
          params: {
            original_image_url: resolvedContext.originalImageUrl,
            ...(maskImageUrl
              ? {
                  mask_image_url: maskImageUrl,
                  use_mask: true,
                }
              : {}),
            target_color: targetColor,
            variation_strength: strength,
            strength,
          },
        }),
        source: placementSource,
        outputNodeId,
        successMessage: `${EXECUTION_DIRECTED_RECOLOR_META.successLabel} submitted`,
        failureMessage: 'Failed to submit color task',
        onCompleted: urls => cacheNodeOutputs(nodeType, node.id, urls),
      })
    } catch (error: unknown) {
      failExecutionFromError(nodeType, node.id, error, 'Failed to submit color task')
    }
  }

  async function buildMaskedImageDataUrl(imageUrl: string, maskDataUrl: string): Promise<string | null> {
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

  async function handleModelNodeGenerate(node: ModelNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('model', node)
    const sourceInput = resolveConnectedModelImageInput(node)
    const modelImageInput = resolveConnectedModelReferenceImageInput(node)
    const promptText = options.resolveNodePromptText('model', node.id, '')
    const missingPorts = options.getMissingRequiredNodeInputs('model', node, {
      image: sourceInput,
      imageA: modelImageInput,
    })
    if (options.warnMissingNodeInputs('model', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'image'
          ? 'Image'
          : port === 'imageA'
            ? 'Model'
            : port
      return formatStrictMissingInputMessage('Model', disconnectedPorts, pendingPorts, formatLabel)
    })) return

    const sourceUrl = sourceInput!.url

    if (!beginExecution('model', node.id)) return
    const angleSpec = { ...node.angleSpec }
    const angleLabel = describeModelAngleSpec(angleSpec)
    const poseImageInput = resolveConnectedModelPoseImageInput(node)
    const modelStrength = clampModelReferenceStrength(node.modelStrength)
    const poseStrength = clampModelPoseStrength(node.poseStrength)

    await submitExecutionTask(
      'model',
      node.id,
      buildStandardSubmitOptions({
        nodeType: 'model',
        nodeId: node.id,
        request: buildExecutionTaskRequest({
          prompt: `${EXECUTION_MODEL_META.promptPrefix} ${promptText}`,
          feature: EXECUTION_MODEL_META.feature,
          imageUrls: [sourceUrl],
          params: {
            angle_spec: {
              yaw: angleSpec.yaw,
              framing: angleSpec.framing,
            },
            ...(modelImageInput?.url ? { model_url: modelImageInput.url, model_strength: modelStrength } : {}),
            ...(poseImageInput?.url ? { pose_url: poseImageInput.url, pose_strength: poseStrength } : {}),
            background: node.background,
          },
        }),
        sourceUrl,
        placementX: node.x + 340,
        placementY: node.y + 18,
        successMessage: `Task submitted · ${angleLabel}`,
        failureMessage: 'Failed to submit task',
      }),
    )
  }

  async function handleViewpointNodeGenerate(node: ViewpointNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('viewpoint', node)
    const connectedImage = resolveConnectedViewpointImageInput(node)
    const promptText = resolveConnectedViewpointPromptInput(node).trim()
    const missingPorts = options.getMissingRequiredNodeInputs('viewpoint', node, {
      image: connectedImage,
    })
    if (options.warnMissingNodeInputs('viewpoint', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = () => 'Image'
      return formatStrictMissingInputMessage('Display', disconnectedPorts, pendingPorts, formatLabel)
    })) return

    if (!node.selectedViewpoint) {
      options.message.warning('请选择视角')
      return
    }

    const sourceUrls = [connectedImage!.url]
    if (!beginExecution('viewpoint', node.id)) return

    await submitExecutionTask(
      'viewpoint',
      node.id,
      buildStandardSubmitOptions({
        nodeType: 'viewpoint',
        nodeId: node.id,
        request: buildExecutionTaskRequest({
          prompt: `${EXECUTION_VIEWPOINT_META.promptPrefix} ${promptText}`,
          feature: EXECUTION_VIEWPOINT_META.feature,
          imageUrls: sourceUrls,
          params: {
            viewpoint: node.selectedViewpoint,
            lighting: node.lighting,
          },
        }),
        sourceUrl: sourceUrls[0],
        placementX: node.x + 340,
        placementY: node.y + 18,
        successMessage: `${EXECUTION_VIEWPOINT_META.successLabel} submitted`,
        failureMessage: 'Failed to submit display task',
      }),
    )
  }

  async function handleEditNodeGenerate(node: EditNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('edit', node)
    const connectedImage = resolveConnectedEditImageInput(node)
    const connectedSourceImage = node.mode === 'face-swap'
      ? resolveConnectedEditSourceImageInput(node)
      : null
    const connectedMask = await resolveConnectedEditMaskInput(node)
    const hasMaskConnection = !!connectionStatus.incoming.mask
    const editModeMeta = node.mode === 'face-swap' ? null : getEditModeMeta(node.mode)
    const promptText = editModeMeta
      ? resolveConnectedEditPromptInput(node)
      : ''
    const missingPorts = options.getMissingRequiredNodeInputs('edit', node, {
      image: connectedImage,
      ...(node.mode === 'face-swap'
        ? { imageB: connectedSourceImage }
        : { prompt: promptText }),
    })
    if (options.warnMissingNodeInputs('edit', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const promptContentMissing = missingPorts.includes('prompt')
        && !!connectionStatus.incoming.prompt
        && !connectionStatus.pendingIncoming.prompt
        && !promptText.trim()
      const formatLabel = (port: string) =>
        port === 'prompt'
          ? 'Prompt（请连接 Prompt 节点并填写内容）'
          : port === 'image' || port === 'imageB' || port === 'mask' || port === 'prompt'
          ? getEditNodePortLabel(node.mode, port)
          : port
      return formatStrictMissingInputMessage('Edit', disconnectedPorts, pendingPorts, formatLabel, promptContentMissing)
    })) return
    const resolvedImage = connectedImage!

    if (!beginExecution('edit', node.id)) return
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('edit', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)

    try {
        if (node.mode === 'face-swap') {
          let faceSourceUrl = connectedSourceImage!.url
          if (connectedMask?.dataUrl) {
            const maskedFaceDataUrl = await buildMaskedImageDataUrl(connectedSourceImage!.url, connectedMask.dataUrl)
            if (!maskedFaceDataUrl) {
              failExecution('edit', node.id, '无法生成分割人脸，请重试')
              return
            }
            const faceFile = await options.dataUrlToFile(maskedFaceDataUrl, `face-swap-face-${Date.now()}.png`)
            const uploadRes = await options.uploadImage(faceFile)
            if (uploadRes.code !== 0 || !uploadRes.data?.url) {
              failExecution('edit', node.id, uploadRes.msg || '分割人脸上传失败')
              return
            }
            faceSourceUrl = uploadRes.data.url
          }

        const placementSource = options.buildPlacementSource(resolvedImage.url, node.x + 340, node.y + 18)

        await submitExecutionTask('edit', node.id, {
          request: buildExecutionTaskRequest({
            prompt: EXECUTION_FACE_SWAP_META.prompt,
            feature: EXECUTION_FACE_SWAP_META.feature,
            imageUrls: [resolvedImage.url, faceSourceUrl],
          }),
          source: placementSource,
          outputNodeId,
          successMessage: connectedMask ? 'Face swap submitted · masked face' : 'Face swap submitted',
          failureMessage: 'Failed to submit edit task',
          onCompleted: urls => cacheNodeOutputs('edit', node.id, urls),
        })
        return
      }

      if (hasMaskConnection && !connectedMask?.dataUrl) {
        options.triggerNodeConnectionError('edit', node.id, ['mask'])
        options.message.warning(
          connectionStatus.pendingIncoming.mask
            ? 'Edit 节点等待上游 Mask 输出'
            : 'Edit 节点蒙版不可用，请检查 Mask 节点',
        )
        return
      }

      let maskImageUrl = ''
      if (connectedMask?.dataUrl) {
        const maskFile = await options.dataUrlToFile(connectedMask.dataUrl, `edit-mask-${Date.now()}.png`)
        const uploadRes = await options.uploadImage(maskFile)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          failExecution('edit', node.id, uploadRes.msg || '蒙版上传失败')
          return
        }
        maskImageUrl = uploadRes.data.url
      }

      const placementSource = options.buildPlacementSource(
        resolvedImage.url,
        connectedMask?.placementX ?? (node.x + 340),
        connectedMask?.placementY ?? (node.y + 18),
      )

      await submitExecutionTask('edit', node.id, {
        request: buildExecutionTaskRequest({
          prompt: `${editModeMeta!.promptPrefix} ${promptText}`,
          feature: editModeMeta!.feature,
          imageUrls: [resolvedImage.url],
          params: {
            original_image_url: resolvedImage.url,
            ...(maskImageUrl
              ? {
                  mask_image_url: maskImageUrl,
                  use_mask: true,
                }
              : {}),
          },
        }),
        source: placementSource,
        outputNodeId,
        successMessage: node.mode === 'bg-swap' ? 'Background swap submitted' : 'Partial edit submitted',
        failureMessage: 'Failed to submit edit task',
        onCompleted: urls => cacheNodeOutputs('edit', node.id, urls),
      })
    } catch (error: unknown) {
      failExecutionFromError('edit', node.id, error, 'Failed to submit edit task')
    }
  }

  async function handleFusionNodeGenerate(node: FusionNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('fusion', node)
    const connectedImageA = resolveConnectedFusionImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedFusionImageInput(node, 'imageB')
    const imageAWeight = node.blendRatio
    const imageBWeight = 100 - node.blendRatio
    const promptText = resolveConnectedFusionPromptInput(node)
    const missingPorts = options.getMissingRequiredNodeInputs('fusion', node, {
      imageA: connectedImageA,
      imageB: connectedImageB,
    })
    if (options.warnMissingNodeInputs('fusion', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'imageA'
          ? 'Image A'
          : 'Image B'
      return formatStrictMissingInputMessage('Fusion', disconnectedPorts, pendingPorts, formatLabel)
    })) return
    if (!beginExecution('fusion', node.id)) return
    await submitExecutionTask(
      'fusion',
      node.id,
      buildStandardSubmitOptions({
        nodeType: 'fusion',
        nodeId: node.id,
        request: buildExecutionTaskRequest({
          prompt: `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`,
          feature: EXECUTION_FUSION_META.feature,
          imageUrls: [connectedImageA!.url, connectedImageB!.url],
          params: {
            blend_ratio: imageAWeight,
            image_a_weight: imageAWeight,
            image_b_weight: imageBWeight,
          },
        }),
        sourceUrl: connectedImageA!.url,
        placementX: node.x + 340,
        placementY: node.y + 18,
        successMessage: `${EXECUTION_FUSION_META.successLabel} submitted`,
        failureMessage: 'Failed to submit fusion task',
      }),
    )
  }

  async function handleEnhanceNodeGenerate(node: EnhanceNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('enhance', node)
    const connectedImage = resolveConnectedEnhanceImageInput(node)
    const connectedMask = node.mode === 'hd-upscale' ? await resolveConnectedEnhanceMaskInput(node) : null
    const hasMaskConnection = node.mode === 'hd-upscale' && !!connectionStatus.incoming.mask
    const promptText = enhanceModeSupportsPrompt(node.mode)
      ? resolveConnectedEnhancePromptInput(node)
      : ''
    const missingPorts = options.getMissingRequiredNodeInputs('enhance', node, {
      image: connectedImage,
    })
    if (options.warnMissingNodeInputs('enhance', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const promptContentMissing = missingPorts.includes('prompt')
        && !!connectionStatus.incoming.prompt
        && !connectionStatus.pendingIncoming.prompt
        && !promptText.trim()
      const formatLabel = (port: string) => port === 'prompt'
        ? 'Prompt（请连接 Prompt 节点并填写内容）'
        : 'Image'
      return formatStrictMissingInputMessage('Enhance', disconnectedPorts, pendingPorts, formatLabel, promptContentMissing)
    })) return

    if (!beginExecution('enhance', node.id)) return
    const outputNodeId = resolveExecutionOutputNodeId('enhance', node.id)
    const placementSource = options.buildPlacementSource(
      connectedImage!.url,
      node.x + 340,
      node.y + 18,
    )

    try {
      if (node.mode === 'hd-upscale') {
        if (hasMaskConnection) {
          if (!connectedMask?.dataUrl) {
            options.triggerNodeConnectionError('enhance', node.id, ['mask'])
            failExecution('enhance', node.id,
              connectionStatus.pendingIncoming.mask
                ? 'Enhance 节点等待上游 Mask 输出'
                : 'Enhance 节点蒙版不可用，请检查 Mask 节点',
            )
            return
          }

          const maskFile = await options.dataUrlToFile(connectedMask.dataUrl, `enhance-mask-${Date.now()}.png`)
          const uploadRes = await options.uploadImage(maskFile)
          if (uploadRes.code !== 0 || !uploadRes.data?.url) {
            failExecution('enhance', node.id, uploadRes.msg || '蒙版上传失败')
            return
          }

          await submitExecutionTask('enhance', node.id, {
            request: buildExecutionTaskRequest({
              prompt: EXECUTION_ENHANCE_META.hdUpscale.prompt,
              feature: EXECUTION_ENHANCE_META.hdUpscale.feature,
              imageUrls: [connectedImage!.url],
              params: {
                original_image_url: connectedImage!.url,
                mask_image_url: uploadRes.data.url,
                use_mask: true,
                mask_enabled: true,
                is_masked: true,
                local_enhance: true,
                upscale_mode: node.upscaleMode,
                upscale_factor: node.upscaleFactor,
                output_format: node.outputFormat,
                output_quality: node.outputQuality,
              },
            }),
            source: placementSource,
            outputNodeId,
            successMessage: 'HD upscale submitted · masked area',
            failureMessage: 'Failed to submit enhance task',
            onCompleted: urls => cacheNodeOutputs('enhance', node.id, urls),
          })
          return
        }

        await submitExecutionTask('enhance', node.id, {
          request: buildExecutionTaskRequest({
            prompt: EXECUTION_ENHANCE_META.hdUpscale.prompt,
            feature: EXECUTION_ENHANCE_META.hdUpscale.feature,
            imageUrls: [connectedImage!.url],
            params: {
              original_image_url: connectedImage!.url,
              upscale_mode: node.upscaleMode,
              upscale_factor: node.upscaleFactor,
              output_format: node.outputFormat,
              output_quality: node.outputQuality,
            },
          }),
          source: placementSource,
          outputNodeId,
          successMessage: 'HD upscale submitted',
          failureMessage: 'Failed to submit enhance task',
          onCompleted: urls => cacheNodeOutputs('enhance', node.id, urls),
        })
        return
      }

      await submitExecutionTask(
        'enhance',
        node.id,
        buildStandardSubmitOptions({
          nodeType: 'enhance',
          nodeId: node.id,
          request: buildExecutionTaskRequest({
            prompt: promptText.trim()
              ? `${EXECUTION_ENHANCE_META.canvasExtend.promptPrefix} ${promptText}`
              : '',
            feature: EXECUTION_ENHANCE_META.canvasExtend.feature,
            imageUrls: [connectedImage!.url],
            params: {
              extend_top: node.extendTop,
              extend_right: node.extendRight,
              extend_bottom: node.extendBottom,
              extend_left: node.extendLeft,
            },
          }),
          sourceUrl: connectedImage!.url,
          placementX: node.x + 340,
          placementY: node.y + 18,
          successMessage: 'Canvas extend submitted',
          failureMessage: 'Failed to submit enhance task',
        }),
      )
    } catch (error: unknown) {
      failExecutionFromError('enhance', node.id, error, 'Failed to enhance image')
    }
  }

  async function handleWearNodeGenerate(node: WearNodeItem) {
    const connectionStatus = options.getNodeConnectionStatus('wear', node)
    const connectedImageA = resolveConnectedWearImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedWearImageInput(node, 'imageB')
    const connectedMask = await resolveConnectedWearMaskInput(node)
    const hasMaskConnection = !!connectionStatus.incoming.mask
    const missingPorts = options.getMissingRequiredNodeInputs('wear', node, {
      imageA: connectedImageA,
      imageB: connectedImageB,
    })

    if (options.warnMissingNodeInputs('wear', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'imageA' ? 'Body' : 'Texture'
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Wear 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Wear 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `Wear 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return

    if (hasMaskConnection && !connectedMask?.dataUrl) {
      options.triggerNodeConnectionError('wear', node.id, ['mask'])
      options.message.warning(
        connectionStatus.pendingIncoming.mask
          ? 'Wear 节点等待上游 Mask 输出'
          : 'Wear 节点蒙版不可用，请检查 Mask 节点',
      )
      return
    }

    if (!beginExecution('wear', node.id)) return
    const tileModeLabel = describeWearTileMode(node.tileMode)
    try {
      let maskImageUrl = ''
      if (connectedMask?.dataUrl) {
        const maskFile = await options.dataUrlToFile(connectedMask.dataUrl, `wear-mask-${Date.now()}.png`)
        const uploadRes = await options.uploadImage(maskFile)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          failExecution('wear', node.id, uploadRes.msg || '蒙版上传失败')
          return
        }
        maskImageUrl = uploadRes.data.url
      }

      await submitExecutionTask(
        'wear',
        node.id,
        buildStandardSubmitOptions({
          nodeType: 'wear',
          nodeId: node.id,
          request: buildExecutionTaskRequest({
            prompt: `${EXECUTION_WEAR_META.promptPrefix} ${tileModeLabel}，旋转${node.rotation}°，大小${node.scale}%，融合强度${node.opacity}%`,
            feature: EXECUTION_WEAR_META.feature,
            imageUrls: [connectedImageA!.url, connectedImageB!.url],
            params: {
              original_image_url: connectedImageA!.url,
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
          }),
          sourceUrl: connectedImageA!.url,
          placementX: node.x + 340,
          placementY: node.y + 18,
          successMessage: `${EXECUTION_WEAR_META.successLabel} submitted`,
          failureMessage: 'Failed to submit wear task',
        }),
      )
    } catch (error: unknown) {
      failExecutionFromError('wear', node.id, error, 'Failed to submit wear task')
    }
  }

  async function handleColorNodeGenerate(node: ColorNodeItem) {
    node.colors = normalizeColorNodeColors(node.colors.map(color => options.normalizeHexColor(color)))
    node.strength = Math.max(0, Math.min(100, Math.round(node.strength ?? DEFAULT_COLOR_STRENGTH)))

    if (node.colors.length === 0) {
      options.message.warning('请至少准备一种颜色')
      return
    }

    if (isColorSingleColorMode(node)) {
      await handleDirectedRecolorGenerate('color', node, node.colors[0], node.strength)
      return
    }

    const resolvedContext = await resolveColorGenerationContext('color', node)
    if (!resolvedContext) return

    if (!beginExecution('color', node.id)) return
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('color', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(
      resolvedContext.originalImageUrl,
      resolvedContext.placementX,
      resolvedContext.placementY,
    )

    try {
      let maskImageUrl = ''
      if (resolvedContext.maskDataUrl) {
        const maskFile = await options.dataUrlToFile(resolvedContext.maskDataUrl, `color-mask-${Date.now()}.png`)
        const uploadRes = await options.uploadImage(maskFile)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          failExecution('color', node.id, uploadRes.msg || '蒙版上传失败')
          return
        }
        maskImageUrl = uploadRes.data.url
      }

      const promptText = resolvedContext.promptText.trim()
      const promptParts = [
        EXECUTION_COLOR_SERIES_META.promptPrefix,
        `目标配色：${node.colors.join(', ')}`,
        `强度：${node.strength}`,
        promptText,
      ].filter(Boolean)

      await submitExecutionTask('color', node.id, {
        request: buildExecutionTaskRequest({
          prompt: promptParts.join('，'),
          feature: EXECUTION_COLOR_SERIES_META.feature,
          imageUrls: [resolvedContext.originalImageUrl],
          params: {
            original_image_url: resolvedContext.originalImageUrl,
            ...(maskImageUrl
              ? {
                  mask_image_url: maskImageUrl,
                  use_mask: true,
                }
              : {}),
            color_type: EXECUTION_COLOR_SERIES_META.colorType,
            colors: node.colors,
            variation_strength: node.strength,
            strength: node.strength,
          },
        }),
        source: placementSource,
        outputNodeId,
        successMessage: 'Color submitted',
        failureMessage: 'Failed to submit color task',
        onCompleted: urls => cacheNodeOutputs('color', node.id, urls),
      })
    } catch (error: unknown) {
      failExecutionFromError('color', node.id, error, 'Failed to submit color task')
    }
  }

  return {
    handleModelNodeGenerate,
    handleViewpointNodeGenerate,
    handleColorNodeGenerate,
    handleEditNodeGenerate,
    handleFusionNodeGenerate,
    handleEnhanceNodeGenerate,
    handleWearNodeGenerate,
  }
}
