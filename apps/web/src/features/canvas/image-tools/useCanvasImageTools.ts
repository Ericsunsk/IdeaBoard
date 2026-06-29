import { computed, provide, readonly } from 'vue'
import {
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  type CanvasImageAdjustmentFinalApplyContext,
} from '../image-editor'
import type { CanvasImageTools } from './imageToolBindings'
import { createImageToolChromeAdapter } from './imageToolChromeAdapter'
import { createImageToolCommands } from './imageToolCommands'
import { createImageToolNodeBindings } from './imageToolNodeBindings'
import {
  createImageToolSession,
  type UseCanvasImageToolsOptions,
} from './imageToolSession'

export type { UseCanvasImageToolsOptions } from './imageToolSession'

export function useCanvasImageTools(options: UseCanvasImageToolsOptions): CanvasImageTools {
  const session = createImageToolSession(options)
  const commands = createImageToolCommands(session)
  const chrome = createImageToolChromeAdapter(session, commands.chromeCommands)
  const nodeBindings = createImageToolNodeBindings(session, chrome.imageEditorRenderedPreview)

  provide<CanvasImageAdjustmentFinalApplyContext>(IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY, {
    progress: readonly(session.imageAdjustmentFinalApplyProgress),
    cancel: () => session.cancelImageAdjustmentFinalTask(),
  })

  const isChromeHidden = computed(() => !!session.imageEditorNodeId.value)
  const isCanvasLocked = computed(
    () => options.isFlowInteractionActive.value || !!session.imageEditorNodeId.value,
  )

  return {
    activeNodeId: session.imageEditorNodeId,
    activeMaskNodeId: session.maskEditor.maskEditorNodeId,
    activeMode: session.imageEditorMode,
    chromeModel: chrome.chromeModel,
    chromeCommands: chrome.chromeCommands,
    isChromeHidden,
    isCanvasLocked,
    isApplyingAdjustments: session.isApplyingImageEditorAdjustments,
    adjustmentFinalApplyProgress: session.imageAdjustmentFinalApplyProgress,
    editorLayers: session.imageEditorSession.imageEditorLayers,
    enter: session.enter,
    cancel: session.cancel,
    commit: session.commitMask,
    apply: commands.applyAdjustments,
    onCanvasWheel: session.viewport.onCanvasWheel,
    focusAndShowChrome: session.viewport.focusImageEditorAndShowChrome,
    scheduleChromeAnchorUpdate: session.viewport.scheduleImageEditorChromeAnchorUpdate,
    stageBindingFor: nodeBindings.stageBindingFor,
    actionBindingFor: nodeBindings.actionBindingFor,
    discardNodeEdits: session.discardNodeEdits,
    runPromptAction: commands.runPromptAction,
  }
}
