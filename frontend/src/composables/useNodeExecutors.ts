import type { Ref } from 'vue'
import type { SubmitTaskData } from '../api/index'
import type {
  CanvasConnectionNodeType,
  CanvasImageItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  TryOnNodeItem,
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
  getIncomingConnections: (nodeType: CanvasConnectionNodeType, nodeId: number) => unknown[]
  triggerNodeConnectionError: (nodeType: CanvasConnectionNodeType, nodeId: number, ports: string[]) => void
  getModelNodeEffectiveSource: (node: ModelNodeItem) => { url: string; ratio: number; connected: boolean }
  getModelNodeConnectedPromptInput: (node: ModelNodeItem) => string
  resolveWorkflowPromptText: (fallback?: string) => string
  getModelNodeConnectedOutputTarget: (node: ModelNodeItem) => ImageNodeItem | null
  getColorNodeConnectedOutputTarget: (node: ColorNodeItem) => ImageNodeItem | null
  getEditNodeConnectedOutputTarget: (node: EditNodeItem) => ImageNodeItem | null
  getFusionNodeConnectedOutputTarget: (node: FusionNodeItem) => ImageNodeItem | null
  getEnhanceNodeConnectedOutputTarget: (node: EnhanceNodeItem) => ImageNodeItem | null
  getTryOnNodeConnectedOutputTarget: (node: TryOnNodeItem) => ImageNodeItem | null
  getPaletteNodeConnectedOutputTarget: (node: PaletteNodeItem) => ImageNodeItem | null
  getGenerateNodeConnectedOutputTarget: (node: GenerateNodeItem) => ImageNodeItem | null
  resolvePreferredOutputNodeId: (connectedOutputNodeId: number | null) => number | null
  buildPlacementSource: (url: string, x: number, y: number) => CanvasImageItem
  warnMissingNodeInputs: (
    nodeType: CanvasConnectionNodeType,
    nodeId: number,
    missingPorts: string[],
    formatMessage: (ports: string[]) => string,
  ) => boolean
  submitTaskAndPoll: (options: SubmitTaskOptionsLike) => Promise<string | null | void>
  appendGeneratedImages: (urls: string[], source?: CanvasImageItem, outputNodeId?: number | null) => void
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
  getConnectedPromptText: (toNodeType: CanvasConnectionNodeType, toNodeId: number, toPort: 'prompt') => string
  buildFullMaskDataUrl: (imageUrl: string) => Promise<string | null>
  refineMaskDataUrl: (maskDataUrl: string, refineAmount: number, invertMask: boolean) => Promise<string>
  dataUrlToFile: (dataUrl: string, fileName: string) => Promise<File>
  uploadImage: (file: File) => Promise<ApiResponseLike<{ url?: string }>>
  upscaleImage: (payload: UpscalePayload) => Promise<ApiResponseLike<{ image_url?: string }>>
  segmentImage: (payload: SegmentPayload) => Promise<ApiResponseLike<{ result_url?: string }>>
  extractDominantColors: (imageUrl: string, maxColors: number) => Promise<string[]>
  paletteModeUsesReference: (mode: PaletteNodeItem['mode']) => boolean
  generateModeRequiresImage: (mode: GenerateNodeItem['mode']) => boolean
  generateModeSupportsStyleModel: (mode: GenerateNodeItem['mode']) => boolean
}

export function useNodeExecutors(options: UseNodeExecutorsOptions) {
  function resolveConnectedColorImageInput(node: ColorNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('color', node.id, 'image')
  }

  async function resolveConnectedColorMaskInput(node: ColorNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('color', node.id, 'mask')
  }

  function resolveConnectedColorPromptInput(node: ColorNodeItem): string {
    return options.getConnectedPromptText('color', node.id, 'prompt')
  }

  function resolveConnectedEditImageInput(node: EditNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('edit', node.id, 'image')
  }

  async function resolveConnectedEditMaskInput(node: EditNodeItem): Promise<PlacedMaskInput | null> {
    return options.resolveConnectedPlacedMaskInput('edit', node.id, 'mask')
  }

  function resolveConnectedEditPromptInput(node: EditNodeItem): string {
    return options.getConnectedPromptText('edit', node.id, 'prompt')
  }

  function resolveConnectedFusionImageInput(node: FusionNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('fusion', node.id, port)
  }

  function resolveConnectedFusionPromptInput(node: FusionNodeItem): string {
    return options.getConnectedPromptText('fusion', node.id, 'prompt')
  }

  function resolveConnectedEnhanceImageInput(node: EnhanceNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('enhance', node.id, 'image')
  }

  function resolveConnectedEnhancePromptInput(node: EnhanceNodeItem): string {
    return options.getConnectedPromptText('enhance', node.id, 'prompt')
  }

  function resolveConnectedTryOnImageInput(node: TryOnNodeItem, port: 'imageA' | 'imageB'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('tryon', node.id, port)
  }

  function resolveConnectedPaletteImageInput(node: PaletteNodeItem, port: 'image' | 'reference'): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('palette', node.id, port)
  }

  function resolveConnectedGenerateImageInput(node: GenerateNodeItem): PlacedImageInput | null {
    return options.resolveConnectedPlacedImageInput('generate', node.id, 'image')
  }

  function resolveConnectedGeneratePromptInput(node: GenerateNodeItem): string {
    return options.getConnectedPromptText('generate', node.id, 'prompt')
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

  async function handleModelNodeGenerate(node: ModelNodeItem) {
    if (options.generating.value) return
    const incomingConnections = options.getIncomingConnections('model', node.id)
    const effectiveSource = options.getModelNodeEffectiveSource(node)
    const sourceUrl = effectiveSource.url
    if (!sourceUrl) {
      options.triggerNodeConnectionError('model', node.id, ['image'])
      options.message.warning(incomingConnections.length > 0 ? 'Model 节点缺少 image 连线' : '请先上传模特图')
      return
    }

    const angles = [...node.selectedAngles]
    if (angles.length === 0) {
      options.message.warning('请至少选择一个角度')
      return
    }

    options.generating.value = true
    const angleLabel = angles.map(options.getModelAngleLabel).join('、')
    const connectedPrompt = options.getModelNodeConnectedPromptInput(node)
    const promptText = incomingConnections.length > 0
      ? (connectedPrompt || `多角度：${angleLabel}`)
      : options.resolveWorkflowPromptText(`多角度：${angleLabel}`)
    const connectedOutputNodeId = options.getModelNodeConnectedOutputTarget(node)?.id ?? null
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
    const incomingConnections = options.getIncomingConnections('color', node.id)
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

    if (incomingConnections.length > 0) {
      const connectedImage = resolveConnectedColorImageInput(node)
      const connectedMask = await resolveConnectedColorMaskInput(node)
      promptPrefix = resolveConnectedColorPromptInput(node)

      const missingPorts = [
        connectedImage ? null : 'image',
        connectedMask ? null : 'mask',
      ].filter((item): item is 'image' | 'mask' => !!item)
      if (options.warnMissingNodeInputs('color', node.id, missingPorts, ports => `Color 节点缺少 ${ports.join(' / ')} 连线`)) return
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
    const connectedOutputNodeId = options.getColorNodeConnectedOutputTarget(node)?.id ?? null
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
            strength,
          },
        },
        source: placementSource,
        outputNodeId,
        successMessage: `Task submitted · ${targetColor} · ${strength}%`,
        failureMessage: 'Failed to submit task',
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
    const promptText = resolveConnectedEditPromptInput(node) || options.editModeMeta[node.mode].fallbackPrompt

    const missingPorts = [
      connectedImage ? null : 'image',
      connectedMask ? null : 'mask',
    ].filter((item): item is 'image' | 'mask' => !!item)
    if (options.warnMissingNodeInputs('edit', node.id, missingPorts, ports => `Edit 节点缺少 ${ports.join(' / ')} 连线`)) return
    const resolvedImage = connectedImage!
    const resolvedMask = connectedMask!

    options.generating.value = true
    const connectedOutputNodeId = options.getEditNodeConnectedOutputTarget(node)?.id ?? null
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
        successMessage: `${node.mode === 'pattern-inpaint' ? 'Pattern' : 'Partial'} edit submitted`,
        failureMessage: 'Failed to submit edit task',
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
    const promptText = resolveConnectedFusionPromptInput(node) || options.fusionModeMeta[node.mode].fallbackPrompt

    const missingPorts = [
      connectedImageA ? null : 'imageA',
      connectedImageB ? null : 'imageB',
    ].filter((item): item is 'imageA' | 'imageB' => !!item)
    if (options.warnMissingNodeInputs('fusion', node.id, missingPorts, ports => `Fusion 节点缺少 ${ports.map(item => item === 'imageA' ? 'Image A' : 'Image B').join(' / ')} 连线`)) return

    options.generating.value = true
    const connectedOutputNodeId = options.getFusionNodeConnectedOutputTarget(node)?.id ?? null
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
    })
  }

  async function handleEnhanceNodeGenerate(node: EnhanceNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedEnhanceImageInput(node)
    const promptText = resolveConnectedEnhancePromptInput(node)

    if (options.warnMissingNodeInputs('enhance', node.id, connectedImage ? [] : ['image'], () => 'Enhance 节点缺少 Image 连线')) return

    options.generating.value = true
    const connectedOutputNodeId = options.getEnhanceNodeConnectedOutputTarget(node)?.id ?? null
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
    const missingPorts = [
      connectedImageA ? null : 'imageA',
      connectedImageB ? null : 'imageB',
    ].filter((item): item is 'imageA' | 'imageB' => !!item)

    if (options.warnMissingNodeInputs('tryon', node.id, missingPorts, ports => `TryOn 节点缺少 ${ports.map(item => item === 'imageA' ? 'Base' : (node.mode === 'pattern-on-body' ? 'Pattern' : 'Fabric')).join(' / ')} 连线`)) return

    options.generating.value = true
    const connectedOutputNodeId = options.getTryOnNodeConnectedOutputTarget(node)?.id ?? null
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
    })
  }

  async function handlePaletteNodeGenerate(node: PaletteNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedPaletteImageInput(node, 'image')
    const useReference = options.paletteModeUsesReference(node.mode)
    const referenceImage = useReference ? resolveConnectedPaletteImageInput(node, 'reference') : null

    const missingPorts = [
      connectedImage ? null : 'image',
      useReference && !referenceImage ? 'reference' : null,
    ].filter((item): item is 'image' | 'reference' => !!item)

    if (options.warnMissingNodeInputs('palette', node.id, missingPorts, ports => `Palette 节点缺少 ${ports.map(item => item === 'reference' ? 'Ref' : 'Image').join(' / ')} 连线`)) return

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
    const connectedOutputNodeId = options.getPaletteNodeConnectedOutputTarget(node)?.id ?? null
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
    })
  }

  async function handleGenerateNodeGenerate(node: GenerateNodeItem) {
    if (options.generating.value) return

    const connectedImage = resolveConnectedGenerateImageInput(node)
    const promptText = resolveConnectedGeneratePromptInput(node)
    const requiresImage = options.generateModeRequiresImage(node.mode)
    const missingPorts = [
      requiresImage && !connectedImage ? 'image' : null,
      promptText ? null : 'prompt',
    ].filter((item): item is 'image' | 'prompt' => !!item)

    if (options.warnMissingNodeInputs('generate', node.id, missingPorts, ports => `Generate 节点缺少 ${ports.map(item => item === 'image' ? 'Image' : 'Prompt').join(' / ')} 连线`)) return

    if (options.generateModeSupportsStyleModel(node.mode) && !node.styleModel) {
      options.message.warning('请先选择 Style')
      return
    }

    options.generating.value = true
    const meta = options.generateModeMeta[node.mode]
    const outputNodeId = options.getGenerateNodeConnectedOutputTarget(node)?.id ?? null
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
    })
  }

  return {
    handleModelNodeGenerate,
    handleColorNodeGenerate,
    handleEditNodeGenerate,
    handleFusionNodeGenerate,
    handleEnhanceNodeGenerate,
    handleTryOnNodeGenerate,
    handlePaletteNodeGenerate,
    handleGenerateNodeGenerate,
  }
}
