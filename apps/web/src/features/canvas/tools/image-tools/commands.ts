import { createRetouchApplyCommands } from '../retouch'
import { useInpaintChromeActions } from '../inpaint'
import type { ImageToolChromeCommands } from './bindings'
import type { ImageToolSession } from './session'

export function createImageToolCommands(session: ImageToolSession) {
  const { execution, ui, feedback } = session.capabilities

  // Retouch apply pipeline lives in retouch (single owner): this seam only
  // delegates so generation-guard / scope semantics stay in one place.
  const { applyRetouch } = createRetouchApplyCommands(session)

  const maskChromeActions = useInpaintChromeActions({
    inpaintNodeId: session.inpaint.inpaintNodeId,
    commitInpaintMode: session.commitMask,
    getCommittedMaskData: session.inpaint.getCommittedMaskData,
    clearActiveNodePanel: ui.clearActiveNodePanel,
    hideStrengthControl: ui.hideStrengthControl,
    runInpaintAction: execution.runInpaintAction,
    onWarning: feedback.warning,
  })

  const chromeCommands: ImageToolChromeCommands = {
    setMaskDrawMode: session.inpaint.setDrawMode,
    setMaskBrushSize: session.inpaint.setBrushSize,
    setMaskLayerOpacity: session.maskChromeState.setInpaintLayerOpacity,
    clearMask: session.inpaint.clearMask,
    undoMask: session.inpaint.undoMask,
    redoMask: session.inpaint.redoMask,
    submitMaskPrompt: maskChromeActions.onInpaintChromeSubmit,
    selectEffectLayer: session.retouchSession.selectImageEffectLayer,
    selectEditorLayer: session.retouchSession.selectRetouchLayer,
    setEditorLayerVisible: session.retouchSession.setRetouchLayerVisible,
    addEditorLayer: session.retouchSession.addRetouchLayer,
    copyEditorLayer: session.retouchSession.copyRetouchLayer,
    mergeEditorLayer: session.retouchSession.mergeRetouchLayer,
    removeEditorLayer: session.retouchSession.removeRetouchLayer,
    updateEditorParam: session.retouchSession.updateSelectedRetouchLayerParam,
    updateEditorRetouchParams: session.retouchSession.updateSelectedRetouchEffectParams,
    applyRetouch,
  }

  return {
    chromeCommands,
    applyRetouch,
  }
}
