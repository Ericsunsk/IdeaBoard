import type { FlowCreatableNodeKind } from './useFlowNodeMutations'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'

const FLOW_NODE_KIND_DRAG_MIME = 'application/x-ideaboard-flow-node-kind'

type UseFlowCanvasDropOptions = {
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  createNodeAtPosition: (
    kind: FlowCreatableNodeKind,
    x: number,
    y: number,
    initialSnapshotPatch?: Record<string, unknown>,
  ) => string | null
  onWarning: (msg: string) => void
  createImageNodeFromFile: (point: { x: number; y: number }, file: File) => Promise<string | null>
  createNodeOptions: ReadonlyArray<{ value?: unknown }>
}

export function useFlowCanvasDrop(options: UseFlowCanvasDropOptions) {
  function onCanvasDragOver(event: DragEvent) {
    if (!event.dataTransfer) return
    event.dataTransfer.dropEffect = 'copy'
  }

  async function onCanvasDrop(event: DragEvent) {
    const point = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const nodeKindRaw = event.dataTransfer?.getData(FLOW_NODE_KIND_DRAG_MIME) || ''
    const nodeKind = nodeKindRaw.trim() as FlowCreatableNodeKind
    if (nodeKind && options.createNodeOptions.some((item) => item.value === nodeKind)) {
      options.createNodeAtPosition(nodeKind, point.x, point.y)
      return
    }

    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    if (!isSupportedUploadImageFile(file)) {
      options.onWarning(unsupportedUploadImageMessage())
      return
    }
    try {
      await options.createImageNodeFromFile(point, file)
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  return {
    onCanvasDragOver,
    onCanvasDrop,
  }
}

export { FLOW_NODE_KIND_DRAG_MIME }
