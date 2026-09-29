import type { PromptAspectRatio } from '../../../board/ports/canvas'
import type { CanvasNodePresentationIntent } from '.'
import type { FlowSnapshotParamPatch } from '../../infrastructure/vue-flow-adapter/flowSnapshotParamMutations.ts'
import type { OutpaintDragAnchor } from '../../application/image-workflow/useImageOutpaintOverlay.ts'

export type FlowCanvasNodePromptSnapshot = {
  readonly text: string
  readonly outputCount?: number | undefined
  readonly aspectRatio?: PromptAspectRatio | undefined
} | null

export type FlowCanvasNodeIntentEmitter = {
  nodeIntent: (intent: CanvasNodePresentationIntent) => void
  nodeMousedown: (nodeId: string, event: MouseEvent) => void
}

export type UseFlowCanvasNodeIntentsOptions = {
  nodeId: () => string
  promptSnapshot: () => FlowCanvasNodePromptSnapshot
  emit: FlowCanvasNodeIntentEmitter
}

type ImageActionRequest = Extract<CanvasNodePresentationIntent, { type: 'action-requested' }>

// Thin intent forwarders: child interactions arrive here and leave as typed
// `node-intent` events with the current node id attached. No business logic.
export function useFlowCanvasNodeIntents(options: UseFlowCanvasNodeIntentsOptions) {
  const { emit } = options
  const nodeId = () => options.nodeId()

  function onPromptTextEdited(text: string) {
    emit.nodeIntent({ type: 'text-edited', nodeId: nodeId(), text })
  }

  function onPromptParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
    key: Key,
    value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
  ) {
    emit.nodeIntent({
      type: 'parameter-changed',
      nodeId: nodeId(),
      patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
    })
  }

  function onImageParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
    key: Key,
    value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
  ) {
    emit.nodeIntent({
      type: 'parameter-changed',
      nodeId: nodeId(),
      patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
    })
  }

  function onImageDeleteRequest() {
    emit.nodeIntent({ type: 'delete-requested', nodeId: nodeId() })
  }

  function onPromptDeleteRequest() {
    emit.nodeIntent({ type: 'delete-requested', nodeId: nodeId() })
  }

  function onImageStageDropped(event: DragEvent) {
    emit.nodeIntent({ type: 'stage-dropped', nodeId: nodeId(), event })
  }

  function onImageFileOpenRequest() {
    emit.nodeIntent({ type: 'file-open-requested', nodeId: nodeId() })
  }

  function onImagePreviewRequest() {
    emit.nodeIntent({ type: 'preview-requested', nodeId: nodeId() })
  }

  function onImageAppendToCanvasRequest() {
    emit.nodeIntent({
      type: 'append-to-canvas-requested',
      nodeId: nodeId(),
    })
  }

  function onImageResetRequest() {
    emit.nodeIntent({ type: 'reset-requested', nodeId: nodeId() })
  }

  function onImageImportRetryRequest() {
    emit.nodeIntent({ type: 'image-import-retry-requested', nodeId: nodeId() })
  }

  function onImageContextMenuRequest(event: MouseEvent) {
    emit.nodeIntent({
      type: 'image-context-menu-requested',
      nodeId: nodeId(),
      event,
    })
  }

  function onPromptMediaInputDrop(event: DragEvent) {
    emit.nodeIntent({
      type: 'media-input-dropped',
      nodeId: nodeId(),
      event,
    })
  }

  function onPromptMediaInputOpen() {
    emit.nodeIntent({
      type: 'media-input-open-requested',
      nodeId: nodeId(),
    })
  }

  function onPromptMediaInputPreview(image: { url: string; ratio: number }) {
    emit.nodeIntent({
      type: 'media-input-preview-requested',
      nodeId: nodeId(),
      image,
    })
  }

  function onPromptMediaInputRemove(image: { id: string }) {
    emit.nodeIntent({
      type: 'media-input-remove-requested',
      nodeId: nodeId(),
      imageId: image.id,
    })
  }

  function onNodeMouseDown(event: MouseEvent) {
    emit.nodeMousedown(nodeId(), event)
  }

  function onImageOutpaintSubmit(prompt: string) {
    emit.nodeIntent({
      type: 'outpaint-submitted',
      nodeId: nodeId(),
      prompt,
    })
  }

  function onFusionParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
    key: Key,
    value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
  ) {
    emit.nodeIntent({
      type: 'parameter-changed',
      nodeId: nodeId(),
      patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
    })
  }

  function onFusionDeleteRequest() {
    emit.nodeIntent({ type: 'delete-requested', nodeId: nodeId() })
  }

  function onFusionMediaInputDrop(event: DragEvent) {
    emit.nodeIntent({
      type: 'media-input-dropped',
      nodeId: nodeId(),
      event,
    })
  }

  function onFusionMediaInputRemove(image: { id: string }) {
    emit.nodeIntent({
      type: 'media-input-remove-requested',
      nodeId: nodeId(),
      imageId: image.id,
    })
  }

  function onFusionContextMenuRequest(event: MouseEvent) {
    emit.nodeIntent({
      type: 'fusion-context-menu-requested',
      nodeId: nodeId(),
      event,
    })
  }

  function onFusionGenerationRequest(
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) {
    emit.nodeIntent({
      type: 'fusion-generation-requested',
      nodeId: nodeId(),
      prompt,
      outputCount,
      aspectRatio,
    })
  }

  function onPromptGenerate() {
    const snapshot = options.promptSnapshot()
    if (!snapshot) return
    emit.nodeIntent({
      type: 'generation-requested',
      nodeId: nodeId(),
      prompt: snapshot.text,
      outputCount: snapshot.outputCount ?? 1,
      aspectRatio: snapshot.aspectRatio ?? 'auto',
    })
  }

  function onImageActionRequest(
    action: ImageActionRequest['action'],
    prompt: string,
    outputCount: number,
    strength?: number,
  ) {
    emit.nodeIntent({
      type: 'action-requested',
      nodeId: nodeId(),
      action,
      prompt,
      outputCount,
      strength,
    })
  }

  function onImageOutpaintHandlePointerDown(payload: {
    anchor: OutpaintDragAnchor
    event: PointerEvent
  }) {
    emit.nodeIntent({
      type: 'outpaint-handle-pointerdown',
      nodeId: nodeId(),
      anchor: payload.anchor,
      event: payload.event,
    })
  }

  function onImageOutpaintHandlePointerMove(event: PointerEvent) {
    emit.nodeIntent({
      type: 'outpaint-handle-pointermove',
      nodeId: nodeId(),
      event,
    })
  }

  function onImageOutpaintHandlePointerUp(event: PointerEvent) {
    emit.nodeIntent({
      type: 'outpaint-handle-pointerup',
      nodeId: nodeId(),
      event,
    })
  }

  function onImageOutpaintFramePointerDown(event: PointerEvent) {
    emit.nodeIntent({
      type: 'outpaint-frame-pointerdown',
      nodeId: nodeId(),
      event,
    })
  }

  return {
    onPromptTextEdited,
    onPromptParameterChanged,
    onImageParameterChanged,
    onImageDeleteRequest,
    onPromptDeleteRequest,
    onImageStageDropped,
    onImageFileOpenRequest,
    onImagePreviewRequest,
    onImageAppendToCanvasRequest,
    onImageResetRequest,
    onImageImportRetryRequest,
    onImageContextMenuRequest,
    onPromptMediaInputDrop,
    onPromptMediaInputOpen,
    onPromptMediaInputPreview,
    onPromptMediaInputRemove,
    onNodeMouseDown,
    onImageOutpaintSubmit,
    onFusionParameterChanged,
    onFusionDeleteRequest,
    onFusionMediaInputDrop,
    onFusionMediaInputRemove,
    onFusionContextMenuRequest,
    onFusionGenerationRequest,
    onPromptGenerate,
    onImageActionRequest,
    onImageOutpaintHandlePointerDown,
    onImageOutpaintHandlePointerMove,
    onImageOutpaintHandlePointerUp,
    onImageOutpaintFramePointerDown,
  }
}
