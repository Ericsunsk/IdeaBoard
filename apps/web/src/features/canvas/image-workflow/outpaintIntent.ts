import type { Ref } from 'vue'
import type { OutpaintDragAnchor } from './useImageOutpaintOverlay'

type UseFlowOutpaintInteractionHandlersOptions = {
  flowOutpaintNodeId: Ref<string | null>
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  commitFlowOutpaintMode: (prompt: string) => Promise<void> | void
  beginFlowOutpaintHandleResize: (event: PointerEvent, anchor: OutpaintDragAnchor) => void
  beginFlowOutpaintFrameDrag: (event: PointerEvent) => void
}

export function useFlowOutpaintInteractionHandlers(options: UseFlowOutpaintInteractionHandlersOptions) {
  function onFlowImageOutpaintSubmit(nodeId: string, prompt: string) {
    if (options.flowOutpaintNodeId.value !== nodeId) return
    options.clearActiveNodePanel()
    options.hideStrengthControl()
    void options.commitFlowOutpaintMode(prompt)
  }

  function onFlowOutpaintHandlePointerDown(
    nodeId: string,
    anchor: OutpaintDragAnchor,
    event: PointerEvent,
  ) {
    if (options.flowOutpaintNodeId.value !== nodeId) return
    options.beginFlowOutpaintHandleResize(event, anchor)
  }

  function onFlowOutpaintFramePointerDown(nodeId: string, event: PointerEvent) {
    if (options.flowOutpaintNodeId.value !== nodeId) return
    options.beginFlowOutpaintFrameDrag(event)
  }

  return {
    onFlowImageOutpaintSubmit,
    onFlowOutpaintHandlePointerDown,
    onFlowOutpaintFramePointerDown,
  }
}
