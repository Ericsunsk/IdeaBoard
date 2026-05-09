import { computed, nextTick, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { resolveConnectedExecutionInput, resolveFlowSourceImageRatio, resolveSourceImageFromNode } from '../services/flowTaskExecutorUtils'
import type { CanvasConnectionNodeType, ImageNodeItem, MaskNodeItem } from '../types/canvasNodes'

type UseFlowNodeMediaInputsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  canvasDrop: {
    readNodeImageDropPayload: (event: DragEvent) => { url: string; ratio: number } | null
    applyImageFileToImageNode: (nodeId: string, file: File) => Promise<void>
    applyImageFileToMaskNode: (nodeId: string, file: File) => Promise<void>
    applyMaskSourceByUrl: (nodeId: string, url: string, ratio?: number) => Promise<void>
  }
  updateSnapshotNode: (nodeId: string, patcher: (prev: Record<string, unknown>) => Record<string, unknown>) => void
  markFlowExecutionNodeDirty: (nodeId: string) => void
  clearMaskCanvas: (nodeId: string) => void
}

export function useFlowNodeMediaInputs(options: UseFlowNodeMediaInputsOptions) {
  function getImageNodeSnapshot(nodeId: string): ImageNodeItem | null {
    const node = (options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>).find(item => item.id === nodeId)
    if (!node || node.data?.kind !== 'image' || !node.data.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    return node.data.snapshotNode as ImageNodeItem
  }

  function getImageNodeDisplayPayload(nodeId: string): { url: string; ratio: number } | null {
    const snapshot = getImageNodeSnapshot(nodeId)
    if (!snapshot) return null
    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
    const url = `${latestOutput?.url || snapshot.sourceImageUrl || ''}`.trim()
    if (!url) return null
    const outputRatio = Number(latestOutput?.ratio)
    const sourceRatio = Number(snapshot.sourceImageRatio)
    return {
      url,
      ratio: Number.isFinite(outputRatio) && outputRatio > 0
        ? outputRatio
        : (Number.isFinite(sourceRatio) && sourceRatio > 0 ? sourceRatio : 1),
    }
  }

  function getMaskNodeSnapshot(nodeId: string): MaskNodeItem | null {
    const node = (options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>).find(item => item.id === nodeId)
    if (!node || node.data?.kind !== 'mask' || !node.data.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    return node.data.snapshotNode as MaskNodeItem
  }

  function resolveFlowImageInputForNode(
    targetNodeId: string,
    port: 'image' | 'mask' | 'prompt',
  ): { sourceKind: CanvasConnectionNodeType; snapshotNode: Record<string, unknown> } | null {
    const connected = resolveConnectedExecutionInput(
      {
        nodes: options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>,
        edges: options.edges.value as Array<{ source: string; sourceHandle?: string | null; target: string; targetHandle?: string | null }>,
      },
      targetNodeId,
      port,
    )
    if (!connected) return null
    return { sourceKind: connected.sourceType, snapshotNode: connected.snapshotNode }
  }


  function getFlowMaskEffectiveSource(nodeId: string): { url: string; ratio: number; hasConnectedImageInput: boolean } {
    const maskNode = getMaskNodeSnapshot(nodeId)
    if (!maskNode) return { url: '', ratio: 1, hasConnectedImageInput: false }
    const imageInput = resolveFlowImageInputForNode(nodeId, 'image')
    if (imageInput) {
      const url = resolveSourceImageFromNode(imageInput.sourceKind, imageInput.snapshotNode).trim()
      if (url) {
        return {
          url,
          ratio: resolveFlowSourceImageRatio(imageInput.sourceKind, imageInput.snapshotNode),
          hasConnectedImageInput: true,
        }
      }
    }
    return {
      url: (maskNode.sourceImageUrl || '').trim(),
      ratio: maskNode.sourceImageRatio > 0 ? maskNode.sourceImageRatio : 1,
      hasConnectedImageInput: false,
    }
  }

  const flowMaskEffectiveSourceCache = computed(() => {
    const cache: Record<string, { url: string; ratio: number; hasConnectedImageInput: boolean }> = {}
    const flowNodes = options.nodes.value as Array<{ id: string; data?: { kind?: string } }>
    for (const node of flowNodes) {
      if (node.data?.kind !== 'mask') continue
      cache[node.id] = getFlowMaskEffectiveSource(node.id)
    }
    return cache
  })

  function getFlowMaskEffectiveSourceCached(nodeId: string): { url: string; ratio: number; hasConnectedImageInput: boolean } {
    return flowMaskEffectiveSourceCache.value[nodeId] || { url: '', ratio: 1, hasConnectedImageInput: false }
  }

  function readNodeImageDropPayload(event: DragEvent): { url: string; ratio: number } | null {
    return options.canvasDrop.readNodeImageDropPayload(event)
  }

  // --- Image node handlers ---
  function onFlowImageOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void options.canvasDrop.applyImageFileToImageNode(nodeId, file)
    }
    input.click()
  }

  function onFlowImageReset(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputImages: [],
    }))
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const payload = readNodeImageDropPayload(event)
    if (payload) {
      options.updateSnapshotNode(nodeId, prev => ({
        ...prev,
        sourceImageUrl: payload.url,
        sourceImageRatio: payload.ratio,
        outputImages: [],
      }))
      options.markFlowExecutionNodeDirty(nodeId)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void options.canvasDrop.applyImageFileToImageNode(nodeId, file)
  }

  // --- Mask node handlers ---
  function onFlowMaskOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void options.canvasDrop.applyImageFileToMaskNode(nodeId, file)
    }
    input.click()
  }

  function onFlowMaskClearImage(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    options.markFlowExecutionNodeDirty(nodeId)
    options.clearMaskCanvas(nodeId)
  }

  function onFlowMaskClearMask(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    options.markFlowExecutionNodeDirty(nodeId)
    options.clearMaskCanvas(nodeId)
  }

  function onFlowMaskStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const payload = readNodeImageDropPayload(event)
    if (payload) {
      void options.canvasDrop.applyMaskSourceByUrl(nodeId, payload.url, payload.ratio)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void options.canvasDrop.applyImageFileToMaskNode(nodeId, file)
  }

  return {
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
    getMaskNodeSnapshot,
    resolveFlowImageInputForNode,
    resolveFlowSourceImageRatio,
    getFlowMaskEffectiveSource,
    getFlowMaskEffectiveSourceCached,
    flowMaskEffectiveSourceCache,
    readNodeImageDropPayload,
    onFlowImageOpenFile,
    onFlowImageReset,
    onFlowImageStageDrop,
    onFlowMaskOpenFile,
    onFlowMaskClearImage,
    onFlowMaskClearMask,
    onFlowMaskStageDrop,
  }
}
