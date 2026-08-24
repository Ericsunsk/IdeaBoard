import type { Ref } from 'vue'
import type { CanvasExtendDragAnchor } from './useImageCanvasExtendOverlay'

type UseFlowExpandInteractionHandlersOptions = {
  flowExpandNodeId: Ref<string | null>
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  commitFlowExpandMode: (prompt: string) => Promise<void> | void
  beginFlowExpandHandleResize: (event: PointerEvent, anchor: CanvasExtendDragAnchor) => void
  beginFlowExpandFrameDrag: (event: PointerEvent) => void
}

export function useFlowExpandInteractionHandlers(options: UseFlowExpandInteractionHandlersOptions) {
  function onFlowImageExpandSubmit(nodeId: string, prompt: string) {
    if (options.flowExpandNodeId.value !== nodeId) return
    options.clearActiveNodePanel()
    options.hideStrengthControl()
    void options.commitFlowExpandMode(prompt)
  }

  function onFlowExpandHandlePointerDown(
    nodeId: string,
    anchor: CanvasExtendDragAnchor,
    event: PointerEvent,
  ) {
    if (options.flowExpandNodeId.value !== nodeId) return
    options.beginFlowExpandHandleResize(event, anchor)
  }

  function onFlowExpandFramePointerDown(nodeId: string, event: PointerEvent) {
    if (options.flowExpandNodeId.value !== nodeId) return
    options.beginFlowExpandFrameDrag(event)
  }

  return {
    onFlowImageExpandSubmit,
    onFlowExpandHandlePointerDown,
    onFlowExpandFramePointerDown,
  }
}
