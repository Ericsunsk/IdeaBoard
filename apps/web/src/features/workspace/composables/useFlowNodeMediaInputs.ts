import type { Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'
import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import { IMAGE_NODE_EMPTY_SIZE } from '../utils/imageNodeSizing'

type UseFlowNodeMediaInputsOptions = {
  nodes: Ref<Node[]>
  canvasDrop: {
    applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  }
  updateSnapshotNode: (
    nodeId: string,
    patcher: (prev: Record<string, unknown>) => Record<string, unknown>,
  ) => void
  markFlowExecutionNodeDirty: (nodeId: string) => void
}

export function useFlowNodeMediaInputs(options: UseFlowNodeMediaInputsOptions) {
  function getImageNodeSnapshot(nodeId: string): ImageNodeItem | null {
    const node = (
      options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
    ).find((item) => item.id === nodeId)
    if (
      !node ||
      node.data?.kind !== 'image' ||
      !node.data.snapshotNode ||
      typeof node.data.snapshotNode !== 'object'
    )
      return null
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
      ratio:
        Number.isFinite(outputRatio) && outputRatio > 0
          ? outputRatio
          : Number.isFinite(sourceRatio) && sourceRatio > 0
            ? sourceRatio
            : 1,
    }
  }

  // --- Image node handlers ---
  function onFlowImageOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = IMAGE_UPLOAD_ACCEPT
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void options.canvasDrop.applyImageFileToImageNode(nodeId, file)
    }
    input.click()
  }

  function onFlowImageReset(nodeId: string) {
    options.updateSnapshotNode(nodeId, (prev) => ({
      ...prev,
      width: IMAGE_NODE_EMPTY_SIZE,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputStatus: 'idle',
      outputImages: [],
    }))
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void options.canvasDrop.applyImageFileToImageNode(nodeId, file)
  }

  return {
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
    onFlowImageOpenFile,
    onFlowImageReset,
    onFlowImageStageDrop,
  }
}
