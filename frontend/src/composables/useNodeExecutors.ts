import type { Ref } from 'vue'
import type { SubmitTaskData } from '../api/index'
import {
  enhanceModeSupportsPrompt,
  paletteModeUsesReference,
} from '../config/canvasNodeConnectionContract'
import type { NodeConnectionStatus } from './useCanvasNodeConnections'
import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeType,
  CanvasImageItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  TryOnNodeItem,
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

type EditModeMeta = Record<EditNodeItem['mode'], { feature: string; promptPrefix: string; fallbackPrompt: string }>
type FusionModeMeta = Record<FusionNodeItem['mode'], { feature: string; promptPrefix: string; fallbackPrompt: string; successLabel: string }>
type TryOnModeMeta = Record<TryOnNodeItem['mode'], { feature: string; promptPrefix: string; successLabel: string }>
type PaletteModeMeta = Record<PaletteNodeItem['mode'], { colorType: string; label: string }>
type GenerateModeMeta = Record<GenerateNodeItem['mode'], { feature: string; promptPrefix: string; successLabel: string }>

interface UseNodeExecutorsOptions {
  generating: Ref<boolean>
  message: MessageApiLike
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  imageNodes: Ref<ImageNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  activeImageNodeId: Ref<number | null>
  activeModelNodeId: Ref<number | null>
  activeMaskNodeId: Ref<number | null>
  paletteReferenceExtractingIds: Ref<number[]>
  modelDefaultPose: string
  modelDefaultExpression: string
  editModeMeta: EditModeMeta
  fusionModeMeta: FusionModeMeta
  tryOnModeMeta: TryOnModeMeta
  paletteModeMeta: PaletteModeMeta
  generateModeMeta: GenerateModeMeta
  getModelAngleLabel: (key: string) => string
  getViewpointLabel: (key: string) => string
  getViewpointLightingLabel: (key: ViewpointNodeItem['lighting']) => string
  getNodeConnectionStatus: (
    nodeType: CanvasConnectionNodeType,
    node: { id: number },
  ) => NodeConnectionStatus
  triggerNodeConnectionError: (nodeType: CanvasConnectionNodeType, nodeId: number, ports: string[]) => void
  getModelNodeEffectiveSource: (node: ModelNodeItem) => { url: string; ratio: number; connected: boolean }
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
    toPort: 'image' | 'imageA' | 'imageB' | 'reference',
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
  extractDominantColors: (imageUrl: string, maxColors: number) => Promise<string[]>
  generateModeSupportsStyleModel: (mode: GenerateNodeItem['mode']) => boolean
}

export function useNodeExecutors(options: UseNodeExecutorsOptions) {
  function cacheNodeOutputs(
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    urls: string[],
  ) {
    options.appendWorkflowNodeOutputs(nodeType, nodeId, urls)
  }

  function resolveConnectedColorImageInput(node: ColorNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('color', node.id, 'image')
  }

  function resolveConnectedMannequinImageInput(node: MannequinNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('mannequin', node.id, 'image')
  }

  function resolveConnectedMannequinPromptInput(node: MannequinNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('mannequin', node.id, fallback)
  }

  function resolveConnectedViewpointImageInput(node: ViewpointNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('viewpoint', node.id, 'image')
  }

  function resolveConnectedViewpointPromptInput(node: ViewpointNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('viewpoint', node.id, fallback)
  }

  async function resolveConnectedColorMaskInput(node: ColorNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('color', node.id, 'mask')
  }

  function resolveConnectedColorPromptInput(node: ColorNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('color', node.id, fallback)
  }

  function resolveConnectedEditImageInput(node: EditNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('edit', node.id, 'image')
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

  function resolveConnectedEnhancePromptInput(node: EnhanceNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('enhance', node.id, fallback)
  }

  function resolveConnectedTryOnImageInput(node: TryOnNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('tryon', node.id, port)
  }

  function resolveConnectedFaceSwapImageInput(node: FaceSwapNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('faceswap', node.id, port)
  }

  async function resolveConnectedFaceSwapMaskInput(node: FaceSwapNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('faceswap', node.id, 'mask')
  }

  function resolveConnectedPaletteImageInput(node: PaletteNodeItem, port: 'image' | 'reference'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('palette', node.id, port)
  }

  function resolveConnectedGenerateImageInput(node: GenerateNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('generate', node.id, 'image')
  }

  function resolveConnectedGeneratePromptInput(node: GenerateNodeItem, fallback = ''): string {
    return options.resolveNodePromptText('generate', node.id, fallback)
  }

  async function resolvePaletteReferenceColors(node: PaletteNodeItem, referenceUrl: string): Promise<string[]> {
    options.paletteReferenceExtractingIds.value = [...new Set([...options.paletteReferenceExtractingIds.value, node.id])]
    try {
      try {
        const segRes = await options.segmentImage({
          image_url: referenceUrl,
          seg_type: 'clothing',
          return_form: 'mask',
        })
        const segUrl = segRes.data?.result_url || segRes.result_url || referenceUrl
        const segmentedColors = await options.extractDominantColors(segUrl, 5)
        if (segmentedColors.length > 0) {
          return segmentedColors
        }
      } catch {
        // fallback below
      }
      return await options.extractDominantColors(referenceUrl, 5)
    } finally {
      options.paletteReferenceExtractingIds.value = options.paletteReferenceExtractingIds.value.filter(item => item !== node.id)
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
    const connectionStatus = options.getNodeConnectionStatus('model', node)
    const effectiveSource = options.getModelNodeEffectiveSource(node)
    const sourceUrl = effectiveSource.url
    if (!sourceUrl) {
      options.triggerNodeConnectionError('model', node.id, ['image'])
      options.message.warning(
        connectionStatus.pendingIncoming.image
          ? 'Model 节点等待上游 Image 输出'
          : connectionStatus.hasAnyConnection
            ? 'Model 节点缺少 image 连线'
            : '请先上传模特图',
      )
      return
    }

    const angles = [...node.selectedAngles]
    if (angles.length === 0) {
      options.message.warning('请至少选择一个角度')
      return
    }

    options.generating.value = true
    const angleLabel = angles.map(options.getModelAngleLabel).join('、')
    const promptText = options.resolveNodePromptText('model', node.id, `多角度：${angleLabel}`)
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('model', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(sourceUrl, node.x + 340, node.y + 18)

    await options.submitTaskAndPoll({
      request: {
        prompt: `[multi-angle] ${promptText}`,
        params: {
          image_urls: [sourceUrl],
          feature: 'multi-angle',
          angles: angles.join(','),
          pose: options.modelDefaultPose,
          expression: options.modelDefaultExpression,
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

  async function handleMannequinNodeGenerate(node: MannequinNodeItem) {
    if (options.generating.value) return
    const connectionStatus = options.getNodeConnectionStatus('mannequin', node)
    const connectedImage = resolveConnectedMannequinImageInput(node)
    const sourceUrl = connectedImage?.url || node.sourceImageUrl
    if (!sourceUrl) {
      options.triggerNodeConnectionError('mannequin', node.id, ['image'])
      options.message.warning(
        connectionStatus.pendingIncoming.image
          ? 'Mannequin 节点等待上游 Image 输出'
          : connectionStatus.hasAnyConnection
            ? 'Mannequin 节点缺少 image 连线'
            : '请先上传人台图',
      )
      return
    }

    options.generating.value = true
    let segmentedImageUrl = ''
    try {
      const segmentRes = await options.segmentImage({
        image_url: sourceUrl,
        seg_type: 'clothing',
      })
      const tempSegmentUrl = segmentRes.data?.result_url || segmentRes.result_url || ''
      if (segmentRes.code !== 0 || !tempSegmentUrl) {
        options.message.error(segmentRes.msg || '服饰分割失败')
        options.generating.value = false
        return
      }

      const saveRes = await options.saveSegmentResult({
        result_url: tempSegmentUrl,
        seg_type: 'clothing',
        feature: 'mannequin-to-model',
      })
      segmentedImageUrl = saveRes.data?.image_url || ''
      if (saveRes.code !== 0 || !segmentedImageUrl) {
        options.message.error(saveRes.msg || '保存分割结果失败')
        options.generating.value = false
        return
      }
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '服饰分割失败')
      options.generating.value = false
      return
    }

    const promptText = resolveConnectedMannequinPromptInput(node, '人台变模特')
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('mannequin', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(sourceUrl, node.x + 340, node.y + 18)

    await options.submitTaskAndPoll({
      request: {
        prompt: `[mannequin-to-model] ${promptText}`,
        params: {
          image_urls: [segmentedImageUrl],
          feature: 'mannequin-to-model',
          ...(promptText ? { user_prompt: promptText } : {}),
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: 'Task submitted · Mannequin',
      failureMessage: 'Failed to submit task',
      onCompleted: urls => cacheNodeOutputs('mannequin', node.id, urls),
    })
  }

  async function handleViewpointNodeGenerate(node: ViewpointNodeItem) {
    if (options.generating.value) return

    const connectionStatus = options.getNodeConnectionStatus('viewpoint', node)
    const connectedImage = resolveConnectedViewpointImageInput(node)
    const sourceUrls = connectedImage
      ? [connectedImage.url]
      : node.sourceImages
          .map(image => image.url)
          .filter((url): url is string => !!url)

    if (sourceUrls.length === 0) {
      options.triggerNodeConnectionError('viewpoint', node.id, ['image'])
      options.message.warning(
        connectionStatus.pendingIncoming.image
          ? 'Viewpoint 节点等待上游 Image 输出'
          : connectionStatus.hasAnyConnection
            ? 'Viewpoint 节点缺少 image 连线'
            : '请先上传服装图或连接 Image 节点',
      )
      return
    }

    if (node.selectedViewpoints.length === 0) {
      options.message.warning('请至少选择一个视角')
      return
    }

    options.generating.value = true
    const viewpointLabels = node.selectedViewpoints.map(options.getViewpointLabel).join('、')
    const lightingLabel = options.getViewpointLightingLabel(node.lighting)
    const summaryPrompt = [
      `视角：${viewpointLabels}`,
      `光影：${lightingLabel}`,
    ].filter(Boolean)
    const promptText = resolveConnectedViewpointPromptInput(node, summaryPrompt.join('，')).trim()
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('viewpoint', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(sourceUrls[0], node.x + 340, node.y + 18)

    await options.submitTaskAndPoll({
      request: {
        prompt: `[virtual-3d] ${promptText}`,
        params: {
          image_urls: sourceUrls,
          feature: 'virtual-3d',
          viewpoints: node.selectedViewpoints.join(','),
          lighting: node.lighting,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: 'Viewpoint submitted',
      failureMessage: 'Failed to submit viewpoint task',
      onCompleted: urls => cacheNodeOutputs('viewpoint', node.id, urls),
    })
  }

  async function handleColorNodeGenerate(node: ColorNodeItem) {
    if (options.generating.value) return

    const targetColor = options.normalizeHexColor(node.targetColor)
    const strength = Math.max(0, Math.min(100, Math.round(node.strength ?? 70)))
    node.targetColor = targetColor
    node.strength = strength
    let originalImageUrl = ''
    let maskDataUrl = ''
    let placementX = node.x + 340
    let placementY = node.y + 18
    const connectionStatus = options.getNodeConnectionStatus('color', node)
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
    const activeModelNode =
      options.activeModelNodeId.value !== null
        ? options.modelNodes.value.find(item => item.id === options.activeModelNodeId.value)
        : null
    const latestModelNode = [...options.modelNodes.value]
      .reverse()
      .find(item => !!item.sourceImageUrl)
    const activeImageNode =
      options.activeImageNodeId.value !== null
        ? options.imageNodes.value.find(item => item.id === options.activeImageNodeId.value)
        : null
    const latestImageNode = [...options.imageNodes.value]
      .reverse()
      .find(item => !!item.sourceImageUrl || item.outputImages.length > 0)
    const latestCanvasImage = [...options.messages.value].reverse()[0] ?? null

    let promptPrefix = ''

    if (connectionStatus.hasAnyConnection) {
      const connectedImage = resolveConnectedColorImageInput(node)
      const connectedMask = await resolveConnectedColorMaskInput(node)
      promptPrefix = resolveConnectedColorPromptInput(node, '')

      const missingPorts = options.getMissingRequiredNodeInputs('color', node, {
        image: connectedImage,
        mask: connectedMask,
      })
      if (options.warnMissingNodeInputs('color', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
        if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
          return `Color 节点等待上游 ${pendingPorts.join(' / ')} 输出`
        }
        if (pendingPorts.length > 0) {
          return `Color 节点缺少 ${disconnectedPorts.join(' / ')} 连线，且等待上游 ${pendingPorts.join(' / ')} 输出`
        }
        return `Color 节点缺少 ${disconnectedPorts.join(' / ')} 连线`
      })) return
      const resolvedImage = connectedImage!
      const resolvedMask = connectedMask!

      originalImageUrl = resolvedImage.url
      maskDataUrl = resolvedMask.dataUrl
      placementX = resolvedMask.placementX
      placementY = resolvedMask.placementY
    } else if (selectedCanvasImage) {
      originalImageUrl = selectedCanvasImage.url
      placementX = selectedCanvasImage.x - 340
      placementY = selectedCanvasImage.y + 18
      const autoMask = await options.buildFullMaskDataUrl(originalImageUrl)
      if (!autoMask) {
        options.message.error('无法生成全图蒙版，请重试')
        return
      }
      maskDataUrl = autoMask
    } else {
      const pickedMaskNode =
        activeMaskNode && activeMaskNode.sourceImageUrl && activeMaskNode.maskDataUrl
          ? activeMaskNode
          : latestMaskNode
      if (pickedMaskNode) {
        originalImageUrl = pickedMaskNode.sourceImageUrl
        maskDataUrl = await options.refineMaskDataUrl(
          pickedMaskNode.maskDataUrl,
          pickedMaskNode.refineAmount ?? 0,
          !!pickedMaskNode.invertMask,
        )
        placementX = pickedMaskNode.x + 340
        placementY = pickedMaskNode.y + 18
      } else {
        const pickedImageNode = options.getImageNodePreferredImage(activeImageNode ?? latestImageNode ?? null)
        const pickedModelNode =
          activeModelNode && activeModelNode.sourceImageUrl
            ? activeModelNode
            : latestModelNode
        const pickedCanvasImage = latestCanvasImage

        if (pickedImageNode) {
          originalImageUrl = pickedImageNode.url
          placementX = (activeImageNode ?? latestImageNode)?.x ?? node.x
          placementY = ((activeImageNode ?? latestImageNode)?.y ?? node.y) + 18
        } else if (pickedModelNode) {
          originalImageUrl = pickedModelNode.sourceImageUrl
          placementX = pickedModelNode.x - 340
          placementY = pickedModelNode.y + 18
        } else if (pickedCanvasImage) {
          originalImageUrl = pickedCanvasImage.url
          placementX = pickedCanvasImage.x - 340
          placementY = pickedCanvasImage.y + 18
        } else {
          options.message.warning('暂无可用输入，请先准备图片或 Mask 节点')
          return
        }

        const autoMask = await options.buildFullMaskDataUrl(originalImageUrl)
        if (!autoMask) {
          options.message.error('无法生成全图蒙版，请重试')
          return
        }
        maskDataUrl = autoMask
      }
    }

    if (!maskDataUrl) {
      options.message.warning('请先准备可用蒙版')
      return
    }

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('color', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    try {
      const maskFile = await options.dataUrlToFile(maskDataUrl, `mask-${Date.now()}.png`)
      const uploadRes = await options.uploadImage(maskFile)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) {
        options.message.error(uploadRes.msg || '蒙版上传失败')
        options.generating.value = false
        return
      }

      const placementSource = options.buildPlacementSource(originalImageUrl, placementX, placementY)

      await options.submitTaskAndPoll({
        request: {
          prompt: `[directed-recolor] ${promptPrefix ? `${promptPrefix}，` : ''}目标颜色：${targetColor}，强度：${strength}`,
          params: {
            image_urls: [uploadRes.data.url],
            original_image_url: originalImageUrl,
            feature: 'directed-recolor',
            target_color: targetColor,
            variation_strength: strength,
            strength,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: `Task submitted · ${targetColor} · ${strength}%`,
        failureMessage: 'Failed to submit task',
        onCompleted: urls => cacheNodeOutputs('color', node.id, urls),
      })
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || 'Failed to submit task')
      options.generating.value = false
    }
  }

  async function handleEditNodeGenerate(node: EditNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedEditImageInput(node)
    const connectedMask = await resolveConnectedEditMaskInput(node)
    const promptText = resolveConnectedEditPromptInput(node, options.editModeMeta[node.mode].fallbackPrompt)
    const missingPorts = options.getMissingRequiredNodeInputs('edit', node, {
      image: connectedImage,
      mask: connectedMask,
    })
    if (options.warnMissingNodeInputs('edit', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Edit 节点等待上游 ${pendingPorts.join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Edit 节点缺少 ${disconnectedPorts.join(' / ')} 连线，且等待上游 ${pendingPorts.join(' / ')} 输出`
      }
      return `Edit 节点缺少 ${disconnectedPorts.join(' / ')} 连线`
    })) return
    const resolvedImage = connectedImage!
    const resolvedMask = connectedMask!

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('edit', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)

    try {
      const maskFile = await options.dataUrlToFile(resolvedMask.dataUrl, `edit-mask-${Date.now()}.png`)
      const uploadRes = await options.uploadImage(maskFile)
      if (uploadRes.code !== 0 || !uploadRes.data?.url) {
        options.message.error(uploadRes.msg || '蒙版上传失败')
        options.generating.value = false
        return
      }

      const placementSource = options.buildPlacementSource(resolvedImage.url, resolvedMask.placementX, resolvedMask.placementY)

      await options.submitTaskAndPoll({
        request: {
          prompt: `${options.editModeMeta[node.mode].promptPrefix} ${promptText}`,
          params: {
            image_urls: [uploadRes.data.url],
            original_image_url: resolvedImage.url,
            feature: options.editModeMeta[node.mode].feature,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage:
          node.mode === 'pattern-inpaint'
            ? 'Pattern edit submitted'
            : node.mode === 'bg-swap'
              ? 'Background swap submitted'
              : 'Partial edit submitted',
        failureMessage: 'Failed to submit edit task',
        onCompleted: urls => cacheNodeOutputs('edit', node.id, urls),
      })
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || 'Failed to submit edit task')
      options.generating.value = false
    }
  }

  async function handleFusionNodeGenerate(node: FusionNodeItem) {
    if (options.generating.value) return

    const connectedImageA = resolveConnectedFusionImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedFusionImageInput(node, 'imageB')
    const promptText = resolveConnectedFusionPromptInput(node, options.fusionModeMeta[node.mode].fallbackPrompt)
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

    await options.submitTaskAndPoll({
      request: {
        prompt: `${options.fusionModeMeta[node.mode].promptPrefix} ${promptText}`,
        feature: options.fusionModeMeta[node.mode].feature,
        image_urls: [connectedImageA!.url, connectedImageB!.url],
        params: {
          feature: options.fusionModeMeta[node.mode].feature,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: `${options.fusionModeMeta[node.mode].successLabel} submitted`,
      failureMessage: 'Failed to submit fusion task',
      onCompleted: urls => cacheNodeOutputs('fusion', node.id, urls),
    })
  }

  async function handleEnhanceNodeGenerate(node: EnhanceNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedEnhanceImageInput(node)
    const promptText = enhanceModeSupportsPrompt(node.mode)
      ? resolveConnectedEnhancePromptInput(node, node.mode === 'deblur' ? '模糊变清晰' : '画布延伸')
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
    const placementSource = options.buildPlacementSource(connectedImage!.url, node.x + 340, node.y + 18)

    try {
      if (node.mode === 'pattern-upscale' || node.mode === 'hd-upscale') {
        const res = await options.upscaleImage({
          image_url: connectedImage!.url,
          mode: node.upscaleMode,
          upscale_factor: node.upscaleFactor,
          output_format: node.outputFormat,
          output_quality: node.outputQuality,
          feature: node.mode,
        })

        if (res.code !== 0 || !res.data?.image_url) {
          options.message.error(res.msg || '增强失败')
          options.generating.value = false
          return
        }

        options.appendGeneratedImages([res.data.image_url], placementSource, outputNodeId)
        cacheNodeOutputs('enhance', node.id, [res.data.image_url])
        options.message.success(node.mode === 'hd-upscale' ? 'HD upscale completed' : 'Pattern upscale completed')
        options.generating.value = false
        return
      }

      await options.submitTaskAndPoll({
        request: {
          prompt: `[${node.mode}] ${promptText || (node.mode === 'deblur' ? '模糊变清晰' : '画布延伸')}`,
          params: {
            image_urls: [connectedImage!.url],
            feature: node.mode,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: node.mode === 'deblur' ? 'Deblur submitted' : 'Canvas extend submitted',
        failureMessage: 'Failed to submit enhance task',
        onCompleted: urls => cacheNodeOutputs('enhance', node.id, urls),
      })
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || 'Failed to enhance image')
      options.generating.value = false
    }
  }

  async function handleTryOnNodeGenerate(node: TryOnNodeItem) {
    if (options.generating.value) return

    const connectedImageA = resolveConnectedTryOnImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedTryOnImageInput(node, 'imageB')
    const missingPorts = options.getMissingRequiredNodeInputs('tryon', node, {
      imageA: connectedImageA,
      imageB: connectedImageB,
    })

    if (options.warnMissingNodeInputs('tryon', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'imageA' ? 'Base' : (node.mode === 'pattern-on-body' ? 'Pattern' : 'Fabric')
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `TryOn 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `TryOn 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `TryOn 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('tryon', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const tileModeLabel = node.tileMode === 'seamless' ? '四方连续' : '非四方连续'
    const placementSource = options.buildPlacementSource(connectedImageA!.url, node.x + 340, node.y + 18)

    await options.submitTaskAndPoll({
      request: {
        prompt: `${options.tryOnModeMeta[node.mode].promptPrefix} ${tileModeLabel}，旋转${node.rotation}°，大小${node.scale}%，透明度${node.opacity}%`,
        params: {
          image_urls: [connectedImageA!.url, connectedImageB!.url],
          feature: options.tryOnModeMeta[node.mode].feature,
          fabric_rotation: node.rotation,
          fabric_opacity: node.opacity,
          fabric_scale: node.scale,
          tile_mode: node.tileMode,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: `${options.tryOnModeMeta[node.mode].successLabel} submitted`,
      failureMessage: 'Failed to submit try-on task',
      onCompleted: urls => cacheNodeOutputs('tryon', node.id, urls),
    })
  }

  async function handleFaceSwapNodeGenerate(node: FaceSwapNodeItem) {
    if (options.generating.value) return

    const connectedImageA = resolveConnectedFaceSwapImageInput(node, 'imageA')
    const connectedImageB = resolveConnectedFaceSwapImageInput(node, 'imageB')
    const connectedMask = await resolveConnectedFaceSwapMaskInput(node)
    const missingPorts = options.getMissingRequiredNodeInputs('faceswap', node, {
      imageA: connectedImageA,
      imageB: connectedImageB,
      mask: connectedMask,
    })

    if (options.warnMissingNodeInputs('faceswap', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'imageA' ? 'Target' : 'Face'
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `FaceSwap 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `FaceSwap 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `FaceSwap 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('faceswap', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(connectedImageA!.url, node.x + 340, node.y + 18)

    try {
      let faceSourceUrl = connectedImageB!.url
      if (connectedMask?.dataUrl) {
        const maskedFaceDataUrl = await buildMaskedImageDataUrl(connectedImageB!.url, connectedMask.dataUrl)
        if (!maskedFaceDataUrl) {
          options.message.error('无法生成分割人脸，请重试')
          options.generating.value = false
          return
        }
        const faceFile = await options.dataUrlToFile(maskedFaceDataUrl, `face-swap-face-${Date.now()}.png`)
        const uploadRes = await options.uploadImage(faceFile)
        if (uploadRes.code !== 0 || !uploadRes.data?.url) {
          options.message.error(uploadRes.msg || '分割人脸上传失败')
          options.generating.value = false
          return
        }
        faceSourceUrl = uploadRes.data.url
      }

      await options.submitTaskAndPoll({
        request: {
          prompt: '[face-swap] 换脸',
          params: {
            image_urls: [connectedImageA!.url, faceSourceUrl],
            feature: 'face-swap',
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: connectedMask ? 'Face swap submitted · masked face' : 'Face swap submitted',
        failureMessage: 'Failed to submit face swap task',
        onCompleted: urls => cacheNodeOutputs('faceswap', node.id, urls),
      })
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || 'Failed to submit face swap task')
      options.generating.value = false
    }
  }

  async function handlePaletteNodeGenerate(node: PaletteNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedPaletteImageInput(node, 'image')
    const useReference = paletteModeUsesReference(node.mode)
    const referenceImage = useReference ? resolveConnectedPaletteImageInput(node, 'reference') : null
    const missingPorts = options.getMissingRequiredNodeInputs('palette', node, {
      image: connectedImage,
      reference: referenceImage,
    })

    if (options.warnMissingNodeInputs('palette', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'reference' ? 'Ref' : 'Image'
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Palette 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Palette 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `Palette 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return

    let colors = [...node.colors]
    if (useReference && referenceImage?.url) {
      colors = await resolvePaletteReferenceColors(node, referenceImage.url)
      if (colors.length > 0) {
        node.colors = [...colors]
      }
    }

    if (colors.length === 0) {
      options.message.warning('请至少准备一种配色')
      return
    }

    options.generating.value = true
    const connectedOutputNodeId = options.getConnectedOutputImageTarget('palette', node.id)?.id ?? null
    const outputNodeId = options.resolvePreferredOutputNodeId(connectedOutputNodeId)
    const placementSource = options.buildPlacementSource(connectedImage!.url, node.x + 340, node.y + 18)
    const promptParts = [
      '系列配色',
      `配色类型：${options.paletteModeMeta[node.mode].label}`,
      `目标配色：${colors.join(', ')}`,
    ]

    await options.submitTaskAndPoll({
      request: {
        prompt: `[series-color] ${promptParts.join('，')}`,
        params: {
          image_urls: [connectedImage!.url],
          feature: 'series-color',
          color_type: options.paletteModeMeta[node.mode].colorType,
          colors,
        },
      },
      source: placementSource,
      outputNodeId,
      successMessage: 'Palette submitted',
      failureMessage: 'Failed to submit palette task',
      onCompleted: urls => cacheNodeOutputs('palette', node.id, urls),
    })
  }

  async function handleGenerateNodeGenerate(node: GenerateNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedGenerateImageInput(node)
    const promptText = resolveConnectedGeneratePromptInput(node, '')
    const missingPorts = options.getMissingRequiredNodeInputs('generate', node, {
      image: connectedImage,
      prompt: promptText,
    })

    if (options.warnMissingNodeInputs('generate', node.id, missingPorts, ({ disconnectedPorts, pendingPorts }) => {
      const formatLabel = (port: string) => port === 'image' ? 'Image' : 'Prompt'
      if (pendingPorts.length > 0 && disconnectedPorts.length === 0) {
        return `Generate 节点等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      if (pendingPorts.length > 0) {
        return `Generate 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线，且等待上游 ${pendingPorts.map(formatLabel).join(' / ')} 输出`
      }
      return `Generate 节点缺少 ${disconnectedPorts.map(formatLabel).join(' / ')} 连线`
    })) return

    if (options.generateModeSupportsStyleModel(node.mode) && !node.styleModel) {
      options.message.warning('请先选择 Style')
      return
    }

    options.generating.value = true
    const meta = options.generateModeMeta[node.mode]
    const outputNodeId = options.getConnectedOutputImageTarget('generate', node.id)?.id ?? null
    const resolvedOutputNodeId = options.resolvePreferredOutputNodeId(outputNodeId)
    const placementSource = options.buildPlacementSource(connectedImage?.url || '', node.x + 340, node.y + 18)

    await options.submitTaskAndPoll({
      request: {
        prompt: meta.promptPrefix ? `${meta.promptPrefix} ${promptText}` : promptText,
        feature: meta.feature,
        ...(connectedImage?.url ? { image_urls: [connectedImage.url] } : {}),
        params: {
          ratio: node.ratio,
          count: node.count,
          quality: node.quality,
          ...(options.generateModeSupportsStyleModel(node.mode) ? { style_model: node.styleModel } : {}),
          feature: meta.feature,
        },
      },
      source: placementSource.url ? placementSource : undefined,
      outputNodeId: resolvedOutputNodeId,
      successMessage: `${meta.successLabel} submitted`,
      failureMessage: 'Failed to submit generate task',
      onCompleted: urls => cacheNodeOutputs('generate', node.id, urls),
    })
  }

  return {
    handleModelNodeGenerate,
    handleMannequinNodeGenerate,
    handleViewpointNodeGenerate,
    handleColorNodeGenerate,
    handleEditNodeGenerate,
    handleFusionNodeGenerate,
    handleEnhanceNodeGenerate,
    handleTryOnNodeGenerate,
    handleFaceSwapNodeGenerate,
    handlePaletteNodeGenerate,
    handleGenerateNodeGenerate,
  }
}
