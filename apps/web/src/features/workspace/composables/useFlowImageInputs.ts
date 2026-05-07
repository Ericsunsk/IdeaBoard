import { nextTick, type Ref } from 'vue'
import { uploadApi } from '../../../shared/api'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { createCanvasImageItemAt } from '../utils/canvasNodeFactory'
import type { FlowCreatableNodeKind } from './useFlowNodeMutations'

type UseFlowImageInputsOptions = {
  isDraggingFile: Ref<boolean>
  currentBoardId: Ref<string | null>
  FLOW_NODE_KIND_DRAG_MIME: string
  FLOW_NODE_IMAGE_DRAG_MIME: string
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  createNodeAtPosition: (kind: FlowCreatableNodeKind, x: number, y: number) => void
  isCreatableKind: (kind: string) => kind is FlowCreatableNodeKind
  appendMessageFlowNode: (item: ReturnType<typeof createCanvasImageItemAt>) => void
  updateSnapshotNode: (nodeId: string, patcher: (snapshotNode: Record<string, any>) => Record<string, any>) => void
  getImageNodeSnapshot: (nodeId: string) => { x: number; y: number; width: number } | null
  getMaskNodeSnapshot: (nodeId: string) => Record<string, any> | null
  renderMaskNodeMask: (nodeId: string) => void
  clearMaskCanvas: (nodeId: string) => void
  loadImageSizeFromUrl: (url: string) => Promise<{ width: number; height: number } | null>
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
}

type NodeImageDropPayload = { url: string; ratio: number }

export function useFlowImageInputs(options: UseFlowImageInputsOptions) {
  function readNodeImageDropPayload(event: DragEvent): NodeImageDropPayload | null {
    const rawUrl = event.dataTransfer?.getData(options.FLOW_NODE_IMAGE_DRAG_MIME)
    if (!rawUrl) return null
    const url = rawUrl.trim()
    if (!url) return null
    const ratioRaw = event.dataTransfer?.getData(`${options.FLOW_NODE_IMAGE_DRAG_MIME}-ratio`) || ''
    const ratio = Number(ratioRaw)
    return {
      url,
      ratio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
    }
  }

  async function loadImageRatioFromFile(file: File): Promise<number | null> {
    const localUrl = URL.createObjectURL(file)
    try {
      const size = await options.loadImageSizeFromUrl(localUrl)
      if (!size || !size.width || !size.height) return null
      return size.height / size.width
    } finally {
      URL.revokeObjectURL(localUrl)
    }
  }

  async function loadImageRatioFromUrl(url: string): Promise<number | null> {
    const size = await options.loadImageSizeFromUrl(url)
    if (!size || !size.width || !size.height) return null
    return size.height / size.width
  }

  async function uploadImageToBoard(file: File): Promise<{ url: string; ratio: number }> {
    if (!file.type.startsWith('image/')) {
      throw new Error('请上传图片文件')
    }
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) {
      throw new Error('缺少 boardId，无法上传图片')
    }
    const ratio = await loadImageRatioFromFile(file)
    const uploadRes = await uploadApi.upload(file, boardId)
    if (uploadRes.code !== 0 || !uploadRes.data?.url) {
      throw new Error(sanitizeApiErrorMessage(uploadRes.msg, '上传失败'))
    }
    return {
      url: uploadRes.data.url,
      ratio: ratio && ratio > 0 ? ratio : 1,
    }
  }

  async function applyImageFileToImageNode(nodeId: string, file: File): Promise<void> {
    const snapshotNode = options.getImageNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Image 节点不存在')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      options.updateSnapshotNode(nodeId, prev => ({
        ...prev,
        sourceImageUrl: uploaded.url,
        sourceImageRatio: uploaded.ratio,
        outputImages: [],
      }))
      options.onSuccess('Image 节点输入已更新')
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  function onFlowImageOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void applyImageFileToImageNode(nodeId, file)
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
  }

  function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void applyImageFileToImageNode(nodeId, file)
  }

  function onFlowMaskOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void applyImageFileToMaskNode(nodeId, file)
    }
    input.click()
  }

  async function applyImageFileToMaskNode(nodeId: string, file: File): Promise<void> {
    const snapshotNode = options.getMaskNodeSnapshot(nodeId)
    if (!snapshotNode) {
      options.onWarning('Mask 节点不存在')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      options.updateSnapshotNode(nodeId, prev => ({
        ...prev,
        sourceImageUrl: uploaded.url,
        sourceImageRatio: uploaded.ratio,
        executionMaskDataUrl: '',
        maskDataUrl: '',
      }))
      void nextTick(() => options.renderMaskNodeMask(nodeId))
      options.onSuccess('Mask 节点输入已更新')
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  function onFlowMaskClearImage(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    options.clearMaskCanvas(nodeId)
  }

  function onFlowMaskClearMask(nodeId: string) {
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    options.clearMaskCanvas(nodeId)
  }

  async function applyMaskSourceByUrl(nodeId: string, url: string, ratio?: number) {
    const nextRatio = Number.isFinite(ratio) && Number(ratio) > 0
      ? Number(ratio)
      : (await loadImageRatioFromUrl(url)) || 1
    options.updateSnapshotNode(nodeId, prev => ({
      ...prev,
      sourceImageUrl: url,
      sourceImageRatio: nextRatio,
      executionMaskDataUrl: '',
      maskDataUrl: '',
    }))
    void nextTick(() => options.renderMaskNodeMask(nodeId))
  }

  function onFlowMaskStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const payload = readNodeImageDropPayload(event)
    if (payload) {
      void applyMaskSourceByUrl(nodeId, payload.url, payload.ratio)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void applyImageFileToMaskNode(nodeId, file)
  }

  function onCanvasDragOver(event: DragEvent) {
    options.isDraggingFile.value = true
    if (!event.dataTransfer) return
    event.dataTransfer.dropEffect = 'copy'
  }

  function onCanvasDragLeave() {
    options.isDraggingFile.value = false
  }

  async function onCanvasDrop(event: DragEvent) {
    options.isDraggingFile.value = false
    const point = options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const nodeKindRaw = event.dataTransfer?.getData(options.FLOW_NODE_KIND_DRAG_MIME) || ''
    const nodeKind = nodeKindRaw.trim()
    if (nodeKind && options.isCreatableKind(nodeKind)) {
      options.createNodeAtPosition(nodeKind, point.x, point.y)
      return
    }

    const payload = readNodeImageDropPayload(event)
    if (payload) {
      const messageNode = createCanvasImageItemAt({ x: point.x, y: point.y }, payload.url, payload.ratio)
      options.appendMessageFlowNode(messageNode)
      return
    }

    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    if (!file.type.startsWith('image/')) {
      options.onWarning('请上传图片文件')
      return
    }
    try {
      const uploaded = await uploadImageToBoard(file)
      const messageNode = createCanvasImageItemAt({ x: point.x, y: point.y }, uploaded.url, uploaded.ratio)
      options.appendMessageFlowNode(messageNode)
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  return {
    onFlowImageOpenFile,
    onFlowImageReset,
    onFlowImageStageDrop,
    onFlowMaskOpenFile,
    onFlowMaskClearImage,
    onFlowMaskClearMask,
    onFlowMaskStageDrop,
    onCanvasDragOver,
    onCanvasDragLeave,
    onCanvasDrop,
  }
}
