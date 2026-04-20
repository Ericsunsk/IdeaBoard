import type { Ref } from 'vue'
import type { SubmitTaskData } from '../api/index'
import { describeModelAngleSpec, describeWearTileMode } from '../components/canvas/nodes/nodeOptionConfig'
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

type PlacedImageInput = { url: string; placementX: number; placementY: number }
type PlacedMaskInput = { dataUrl: string; placementX: number; placementY: number }
type UpscalePayload = {
  image_url: string
  mode: string
  upscale_factor: number
  output_format: string
  output_quality: number
  feature?: string
}
type SegmentPayload = {
  image_url: string
  seg_type: string
  return_form?: string
}

type StandardEditMode = Exclude<EditNodeItem['mode'], 'face-swap'>
type EditModeMeta = Record<StandardEditMode, { feature: string; promptPrefix: string; fallbackPrompt: string }>
type WearFeatureMeta = { feature: string; promptPrefix: string; successLabel: string }

const FUSION_META = {
  feature: 'style-fusion',
  promptPrefix: '[style-fusion]',
  fallbackPrompt: '双图融合',
  successLabel: 'Fusion',
} as const

const COLOR_SERIES_META = {
  feature: 'series-color',
  colorType: 'reference',
  promptPrefix: '[series-color] 多色配色',
} as const

const DIRECTED_RECOLOR_META = {
  feature: 'directed-recolor',
  promptPrefix: '[directed-recolor]',
  successLabel: 'Color',
} as const

interface UseNodeExecutorsOptions {
  generating: Ref<boolean>
  message: MessageApiLike
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  imageNodes: Ref<ImageNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  activeImageNodeId: Ref<number | null>
  activeMaskNodeId: Ref<number | null>
  editModeMeta: EditModeMeta
  wearFeatureMeta: WearFeatureMeta
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
  refineMaskDataUrl: (maskDataUrl: string, refineAmount: number, invertMask: boolean) => Promise<string>
  dataUrlToFile: (dataUrl: string, fileName: string) => Promise<File>
  uploadImage: (file: File) => Promise<ApiResponseLike<{ url?: string }>>
  upscaleImage: (payload: UpscalePayload) => Promise<ApiResponseLike<{ image_url?: string }>>
  segmentImage: (payload: SegmentPayload) => Promise<ApiResponseLike<{ result_url?: string }>>
  saveSegmentResult: (payload: { result_url: string; seg_type: string; feature: string }) => Promise<ApiResponseLike<{ image_url?: string }>>
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
}

export function useNodeExecutors(options: UseNodeExecutorsOptions) {
  function getEditModeMeta(mode: StandardEditMode) {
    return options.editModeMeta[mode]
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

  function failExecution(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    message: string,
  ) {
    options.message.error(message)
    markExecutionFailed(nodeType, nodeId)
    options.generating.value = false
  }

  function failExecutionFromError(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    error: unknown,
    fallback: string,
  ) {
    failExecution(nodeType, nodeId, resolveFailureMessage(error, fallback))
  }

  function cacheNodeOutputs(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    urls: string[],
  ) {
    options.appendWorkflowNodeOutputs(nodeType, nodeId, urls)
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

  function resolveConnectedViewpointPromptInput(node: ViewpointNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('viewpoint', node.id, fallback)
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

  function resolveConnectedEditPromptInput(node: EditNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('edit', node.id, fallback)
  }

  function resolveConnectedFusionImageInput(node: FusionNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('fusion', node.id, port)
  }

  function resolveConnectedFusionPromptInput(node: FusionNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('fusion', node.id, fallback)
  }

  function resolveConnectedEnhanceImageInput(node: EnhanceNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('enhance', node.id, 'image')
  }

  async function resolveConnectedEnhanceMaskInput(node: EnhanceNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('enhance', node.id, 'mask')
  }

  function resolveConnectedEnhancePromptInput(node: EnhanceNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('enhance', node.id, fallback)
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
    let originalImageUrl = ''
    let maskDataUrl = ''
    let placementX = node.x + 340
    let placementY = node.y + 18
    const connectionStatus = options.getNodeConnectionStatus(nodeType, node)
    const selectedCanvasImage =
      options.selectedIds.value.length === 1
        ? options.messages.value.find(item => item.id === options.selectedIds.value[0])
        : null
    const activeMaskNode =
      options.activeMaskNodeId.value !== null
        ? options.maskNodes.value.find(item => item.id === options.activeMaskNodeId.value)
        : null
    const latestMaskNode = [...options.maskNodes.value]
      .reverse()
      .find(item => !!item.sourceImageUrl && !!item.maskDataUrl)
    const activeImageNode =
      options.activeImageNodeId.value !== null
        ? options.imageNodes.value.find(item => item.id === options.activeImageNodeId.value)
        : null
    const latestImageNode = [...options.imageNodes.value]
      .reverse()
      .find(item => !!item.sourceImageUrl || item.outputImages.length > 0)
    const latestCanvasImage = [...options.messages.value].reverse()[0] ?? null
    const promptText = options.resolveNodePromptText(nodeType, node.id, '')

    if (connectionStatus.hasAnyConnection) {
      const connectedImage = options.resolveConnectedPlacedImageInput(nodeType, node.id, 'image')
      const connectedMask = await options.resolveConnectedPlacedMaskInput(nodeType, node.id, 'mask')
      const missingPorts = options.getMissingRequiredNodeInputs(nodeType, node, {
        image: connectedImage,
        mask: connectedMask,
      })
      if (options.warnMissingNodeInputs(nodeType, node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
        if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
          return `Color 节点等待上游 ${pendingPorts.join(' / ')} 输出`
        }
        if (pendingPorts.length > 0) {
          return `Color 节点缺少 ${disconnectedPorts.join(' / ')} 连线，且等待上游 ${pendingPorts.join(' / ')} 输出`
        }
        return `Color 节点缺少 ${disconnectedPorts.join(' / ')} 连线`
      })) return null

      const resolvedImage = connectedImage!
      const resolvedMask = connectedMask!

      return {
        originalImageUrl: resolvedImage.url,
        maskDataUrl: resolvedMask.dataUrl,
        placementX: resolvedMask.placementX,
        placementY: resolvedMask.placementY,
        promptText,
      }
    }

    if (selectedCanvasImage) {
      originalImageUrl = selectedCanvasImage.url
      placementX = selectedCanvasImage.x - 340
      placementY = selectedCanvasImage.y + 18
      const autoMask = await options.buildFullMaskDataUrl(originalImageUrl)
      if (!autoMask) {
        options.message.error('无法生成全图蒙版，请重试')
        return null
      }
      return {
        originalImageUrl,
        maskDataUrl: autoMask,
        placementX,
        placementY,
        promptText,
      }
    }

    const pickedMaskNode =
      activeMaskNode && activeMaskNode.sourceImageUrl && activeMaskNode.maskDataUrl
        ? activeMaskNode
        : latestMaskNode
    if (pickedMaskNode) {
      maskDataUrl = await options.refineMaskDataUrl(
        pickedMaskNode.maskDataUrl,
        pickedMaskNode.refineAmount ?? 0,
        !!pickedMaskNode.invertMask,
      )
      return {
        originalImageUrl: pickedMaskNode.sourceImageUrl,
        maskDataUrl,
        placementX: pickedMaskNode.x + 340,
        placementY: pickedMaskNode.y + 18,
        promptText,
      }
    }

    const pickedImageNode = options.getImageNodePreferredImage(activeImageNode ?? latestImageNode ?? null)
    if (pickedImageNode) {
      originalImageUrl = pickedImageNode.url
      placementX = (activeImageNode ?? latestImageNode)?.x ?? node.x
      placementY = ((activeImageNode ?? latestImageNode)?.y ?? node.y) + 18
    } else if (latestCanvasImage) {
      originalImageUrl = latestCanvasImage.url
      placementX = latestCanvasImage.x - 340
      placementY = latestCanvasImage.y + 18
    } else {
      options.message.warning('暂无可用输入，请先准备图片或 Mask 节点')
      return null
    }

    const autoMask = await options.buildFullMaskDataUrl(originalImageUrl)
    if (!autoMask) {
      options.message.error('无法生成全图蒙版，请重试')
      return null
    }
    return {
      originalImageUrl,
      maskDataUrl: autoMask,
      placementX,
      placementY,
      promptText,
    }
  }

  async function handleDirectedRecolorGenerate(
    nodeType: 'color',
    node: { id: number; x: number; y: number },
    targetColor: string,
    strength: number,
  ) {
    if (options.generating.value) return
    const resolvedContext = await resolveColorGenerationContext(nodeType, node)
    if (!resolvedContext) return
    if (!resolvedContext.maskDataUrl) {
      options.message.warning('请先准备可用蒙版')
      return
    }

    markExecutionRunning(nodeType, node.id)
    options.generating.value = true
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

      await submitExecutionTask(nodeType, node.id, {
        request: {
          prompt: `${DIRECTED_RECOLOR_META.promptPrefix} ${resolvedContext.promptText ? `${resolvedContext.promptText}，` : ''}目标颜色：${targetColor}，强度：${strength}`,
          params: {
            image_urls: [uploadRes.data.url],
            original_image_url: resolvedContext.originalImageUrl,
            feature: DIRECTED_RECOLOR_META.feature,
            target_color: targetColor,
            variation_strength: strength,
            strength,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: `${DIRECTED_RECOLOR_META.successLabel} submitted`,
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
    if (options.generating.value) return
    const sourceInput = resolveConnectedModelImageInput(node)
    const modelImageInput = resolveConnectedModelReferenceImageInput(node)
    const missingPorts = options.getMissingRequiredNodeInputs('model', node, {
      image: sourceInput,
      imageA: modelImageInput,
    })
    if (options.warnMissingNodeInputs('model', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Model 节点等待上游 ${pendingPorts.join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Model 节点缺少 ${disconnectedPorts.join(' / ')} 连线，且等待上游 ${pendingPorts.join(' / ')} 输出`
      }
      return `Model 节点缺少 ${disconnectedPorts.join(' / ')} 连线`
    })) return

    const sourceUrl = sourceInput!.url

    options.generating.value = true
    const angleSpec = { ...node.angleSpec }
    const angleLabel = describeModelAngleSpec(angleSpec)
    const promptText = options.resolveNodePromptText('model', node.id, `视角：${angleLabel}`)
    const poseImageInput = resolveConnectedModelPoseImageInput(node)
    const modelStrength = clampModelReferenceStrength(node.modelStrength)
    const poseStrength = clampModelPoseStrength(node.poseStrength)
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('model', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(sourceUrl, node.x + 340, node.y + 18)

    await submitExecutionTask('model', node.id, {
      request: {
        prompt: `[multi-angle] ${promptText}`,
        params: {
          image_urls: [sourceUrl],
          feature: 'multi-angle',
          angle_spec: {
            yaw: angleSpec.yaw,
            framing: angleSpec.framing,
          },
          ...(modelImageInput?.url ? { model_url: modelImageInput.url, model_strength: modelStrength } : {}),
          ...(poseImageInput?.url ? { pose_url: poseImageInput.url, pose_strength: poseStrength } : {}),
          background: node.background,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: `Task submitted · ${angleLabel}`,
      failureMessage: 'Failed to submit task',
      onCompleted: urls => cacheNodeOutputs('model', node.id, urls),
    })
  }

  async function handleViewpointNodeGenerate(node: ViewpointNodeItem) {
    if (options.generating.value) return

    const connectionStatus = options.getNodeConnectionStatus('viewpoint', node)
    const connectedImage = resolveConnectedViewpointImageInput(node)
    const sourceUrls = connectedImage ? [connectedImage.url] : []

    if (sourceUrls.length === 0) {
      options.triggerNodeConnectionError('viewpoint', node.id, ['image'])
      options.message.warning(
        connectionStatus.pendingIncoming.image
          ? 'Display 节点等待上游 Image 输出'
          : connectionStatus.hasAnyConnection
            ? 'Display 节点缺少 image 连线'
            : '请先连接 Image 节点',
      )
      return
    }

    if (!node.selectedViewpoint) {
      options.message.warning('请选择视角')
      return
    }

    options.generating.value = true
    const viewpointLabel = options.getViewpointLabel(node.selectedViewpoint)
    const lightingLabel = options.getViewpointLightingLabel(node.lighting)
    const summaryPrompt = [
      `视角：${viewpointLabel}`,
      `光影：${lightingLabel}`,
    ].filter(Boolean)
    const promptText = resolveConnectedViewpointPromptInput(node, summaryPrompt.join('，')).trim()
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('viewpoint', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(sourceUrls[0], node.x + 340, node.y + 18)

    await submitExecutionTask('viewpoint', node.id, {
      request: {
        prompt: `[virtual-3d] ${promptText}`,
        params: {
          image_urls: sourceUrls,
          feature: 'virtual-3d',
          viewpoint: node.selectedViewpoint,
          lighting: node.lighting,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: 'Display submitted',
      failureMessage: 'Failed to submit display task',
      onCompleted: urls => cacheNodeOutputs('viewpoint', node.id, urls),
    })
  }

  async function handleEditNodeGenerate(node: EditNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedEditImageInput(node)
    const connectedSourceImage = node.mode === 'face-swap'
      ? resolveConnectedEditSourceImageInput(node)
      : null
    const connectedMask = await resolveConnectedEditMaskInput(node)
    const editModeMeta = node.mode === 'face-swap' ? null : getEditModeMeta(node.mode)
    const promptText = editModeMeta
      ? resolveConnectedEditPromptInput(node, editModeMeta.fallbackPrompt)
      : ''
    const missingPorts = options.getMissingRequiredNodeInputs('edit', node, {
      image: connectedImage,
      ...(node.mode === 'face-swap'
        ? { imageB: connectedSourceImage }
        : { mask: connectedMask }),
    })
    if (options.warnMissingNodeInputs('edit', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) =>
        port === 'image' || port === 'imageB' || port === 'mask' || port === 'prompt'
          ? getEditNodePortLabel(node.mode, port)
          : port
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Edit 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Edit 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `Edit 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return
    const resolvedImage = connectedImage!

    options.generating.value = true
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
          request: {
            prompt: '[face-swap] 换脸',
            params: {
              image_urls: [resolvedImage.url, faceSourceUrl],
              feature: 'face-swap',
            },
          },
          source: placementSource,
          outputNodeId,
          successMessage: connectedMask ? 'Face swap submitted · masked face' : 'Face swap submitted',
          failureMessage: 'Failed to submit edit task',
          onCompleted: urls => cacheNodeOutputs('edit', node.id, urls),
        })
        return
      }

      const resolvedMask = connectedMask!
      const maskFile = await options.dataUrlToFile(resolvedMask.dataUrl, `edit-mask-${Date.now()}.png`)
      const uploadRes = await options.uploadImage(maskFile)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) {
        failExecution('edit', node.id, uploadRes.msg || '蒙版上传失败')
        return
      }

      const placementSource = options.buildPlacementSource(resolvedImage.url, resolvedMask.placementX, resolvedMask.placementY)

      await submitExecutionTask('edit', node.id, {
        request: {
          prompt: `${editModeMeta!.promptPrefix} ${promptText}`,
          params: {
            image_urls: [uploadRes.data.url],
            original_image_url: resolvedImage.url,
            feature: editModeMeta!.feature,
          },
        },
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
    if (options.generating.value) return

    const connectedImageA = resolveConnectedFusionImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedFusionImageInput(node, 'imageB')
    const imageAWeight = node.blendRatio
    const imageBWeight = 100 - node.blendRatio
    const promptText = resolveConnectedFusionPromptInput(node, FUSION_META.fallbackPrompt)
    const missingPorts = options.getMissingRequiredNodeInputs('fusion', node, {
      imageA: connectedImageA,
      imageB: connectedImageB,
    })
    if (options.warnMissingNodeInputs('fusion', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'imageA' ? 'Image A' : 'Image B'
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Fusion 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Fusion 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `Fusion 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return
    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('fusion', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(connectedImageA!.url, node.x + 340, node.y + 18)

    await submitExecutionTask('fusion', node.id, {
      request: {
        prompt: `${FUSION_META.promptPrefix} ${promptText}`,
        feature: FUSION_META.feature,
        image_urls: [connectedImageA!.url, connectedImageB!.url],
        params: {
          feature: FUSION_META.feature,
          blend_ratio: imageAWeight,
          image_a_weight: imageAWeight,
          image_b_weight: imageBWeight,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: `${FUSION_META.successLabel} submitted`,
      failureMessage: 'Failed to submit fusion task',
      onCompleted: urls => cacheNodeOutputs('fusion', node.id, urls),
    })
  }

  async function handleEnhanceNodeGenerate(node: EnhanceNodeItem) {
    if (options.generating.value) return

    const connectionStatus = options.getNodeConnectionStatus('enhance', node)
    const connectedImage = resolveConnectedEnhanceImageInput(node)
    const connectedMask = node.mode === 'hd-upscale' ? await resolveConnectedEnhanceMaskInput(node) : null
    const hasMaskConnection = node.mode === 'hd-upscale' && !!connectionStatus.incoming.mask
    const promptText = enhanceModeSupportsPrompt(node.mode)
      ? resolveConnectedEnhancePromptInput(node, '画布延伸')
      : ''
    const missingPorts = options.getMissingRequiredNodeInputs('enhance', node, {
      image: connectedImage,
      ...(enhanceModeSupportsPrompt(node.mode) ? { prompt: promptText } : {}),
    })
    if (options.warnMissingNodeInputs('enhance', node.id, missingPorts, ({ pendingPorts }) => (
      pendingPorts.length > 0
        ? 'Enhance 节点等待上游 Image 输出'
        : 'Enhance 节点缺少 Image 连线'
    ))) return

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('enhance', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
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
            request: {
              prompt: '[hd-upscale] 局部高清放大，仅增强蒙版区域',
              params: {
                image_urls: [connectedImage!.url],
                original_image_url: connectedImage!.url,
                mask_image_url: uploadRes.data.url,
                feature: 'hd-upscale',
                use_mask: true,
                mask_enabled: true,
                is_masked: true,
                local_enhance: true,
                upscale_mode: node.upscaleMode,
                upscale_factor: node.upscaleFactor,
                output_format: node.outputFormat,
                output_quality: node.outputQuality,
              },
            },
            source: placementSource,
            outputNodeId,
            successMessage: 'HD upscale submitted · masked area',
            failureMessage: 'Failed to submit enhance task',
            onCompleted: urls => cacheNodeOutputs('enhance', node.id, urls),
          })
          return
        }

        markExecutionRunning('enhance', node.id)
        const res = await options.upscaleImage({
          image_url: connectedImage!.url,
          mode: node.upscaleMode,
          upscale_factor: node.upscaleFactor,
          output_format: node.outputFormat,
          output_quality: node.outputQuality,
          feature: 'hd-upscale',
        })

        if (res.code !== 0 || !res.data?.image_url) {
          failExecution('enhance', node.id, res.msg || '增强失败')
          return
        }

        options.appendGeneratedImages([res.data.image_url], placementSource, outputNodeId)
        cacheNodeOutputs('enhance', node.id, [res.data.image_url])
        options.message.success('HD upscale completed')
        markExecutionSucceeded('enhance', node.id)
        options.generating.value = false
        return
      }

      await submitExecutionTask('enhance', node.id, {
        request: {
          prompt: `[canvas-extend] ${promptText || '画布延伸'}`,
          params: {
            image_urls: [connectedImage!.url],
            feature: 'canvas-extend',
            extend_top: node.extendTop,
            extend_right: node.extendRight,
            extend_bottom: node.extendBottom,
            extend_left: node.extendLeft,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: 'Canvas extend submitted',
        failureMessage: 'Failed to submit enhance task',
        onCompleted: urls => cacheNodeOutputs('enhance', node.id, urls),
      })
    } catch (error: unknown) {
      failExecutionFromError('enhance', node.id, error, 'Failed to enhance image')
    }
  }

  async function handleWearNodeGenerate(node: WearNodeItem) {
    if (options.generating.value) return

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

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('wear', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const tileModeLabel = describeWearTileMode(node.tileMode)
    const placementSource = options.buildPlacementSource(connectedImageA!.url, node.x + 340, node.y + 18)
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

      await submitExecutionTask('wear', node.id, {
        request: {
          prompt: `${options.wearFeatureMeta.promptPrefix} ${tileModeLabel}，旋转${node.rotation}°，大小${node.scale}%，融合强度${node.opacity}%`,
          params: {
            image_urls: [connectedImageA!.url, connectedImageB!.url],
            original_image_url: connectedImageA!.url,
            ...(maskImageUrl
              ? {
                  mask_image_url: maskImageUrl,
                  use_mask: true,
                }
              : {}),
            feature: options.wearFeatureMeta.feature,
            fabric_rotation: node.rotation,
            fabric_opacity: node.opacity,
            fabric_scale: node.scale,
            tile_mode: node.tileMode,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: `${options.wearFeatureMeta.successLabel} submitted`,
        failureMessage: 'Failed to submit wear task',
        onCompleted: urls => cacheNodeOutputs('wear', node.id, urls),
      })
    } catch (error: unknown) {
      failExecutionFromError('wear', node.id, error, 'Failed to submit wear task')
    }
  }

  async function handleColorNodeGenerate(node: ColorNodeItem) {
    if (options.generating.value) return

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
    if (!resolvedContext.maskDataUrl) {
      options.message.warning('请先准备可用蒙版')
      return
    }

    options.generating.value = true
    markExecutionRunning('color', node.id)
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('color', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(
      resolvedContext.originalImageUrl,
      resolvedContext.placementX,
      resolvedContext.placementY,
    )

    try {
      const maskFile = await options.dataUrlToFile(resolvedContext.maskDataUrl, `color-mask-${Date.now()}.png`)
      const uploadRes = await options.uploadImage(maskFile)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) {
        failExecution('color', node.id, uploadRes.msg || '蒙版上传失败')
        return
      }

      const promptText = resolvedContext.promptText.trim()
      const promptParts = [
        COLOR_SERIES_META.promptPrefix,
        `目标配色：${node.colors.join(', ')}`,
        `强度：${node.strength}`,
        promptText,
      ].filter(Boolean)

      await submitExecutionTask('color', node.id, {
        request: {
          prompt: promptParts.join('，'),
          params: {
            image_urls: [uploadRes.data.url],
            original_image_url: resolvedContext.originalImageUrl,
            feature: COLOR_SERIES_META.feature,
            color_type: COLOR_SERIES_META.colorType,
            colors: node.colors,
            variation_strength: node.strength,
            strength: node.strength,
          },
        },
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
