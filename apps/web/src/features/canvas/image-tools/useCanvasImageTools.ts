import { computed } from 'vue'
import type { CanvasImageTools } from './imageToolBindings'
import { createImageToolChromeAdapter } from './imageToolChromeAdapter'
import { createImageToolCommands } from './imageToolCommands'
import { createImageToolNodeBindings } from './imageToolNodeBindings'
import { createImageToolSession, type UseCanvasImageToolsOptions } from './imageToolSession'

export type { UseCanvasImageToolsOptions } from './imageToolSession'

export function useCanvasImageTools(options: UseCanvasImageToolsOptions): CanvasImageTools {
  const session = createImageToolSession(options)
  const commands = createImageToolCommands(session)
  const chrome = createImageToolChromeAdapter(session, commands.chromeCommands)
  const nodeBindings = createImageToolNodeBindings(session, chrome.imageEditorRenderedPreview)

  const isChromeHidden = computed(() => !!session.imageEditorNodeId.value)
  const isCanvasLocked = computed(
    () => options.isCanvasInteractionLocked.value || !!session.imageEditorNodeId.value,
  )

  return {
    viewModel: {
      active: {
        nodeId: session.imageEditorNodeId,
        maskNodeId: session.maskEditor.maskEditorNodeId,
        mode: session.imageEditorMode,
      },
      locks: {
        isChromeHidden,
        isCanvasLocked,
      },
      chrome: chrome.chromeModel,
      adjustment: {
        isApplying: session.isApplyingImageEditorAdjustments,
      },
      nodeBindings: {
        stageFor: nodeBindings.stageBindingFor,
        actionFor: nodeBindings.actionBindingFor,
      },
    },
    commands: {
      lifecycle: {
        enter: session.enter,
        cancel: session.cancel,
        commit: session.commitMask,
        discardNodeEdits: session.discardNodeEdits,
      },
      editor: {
        apply: commands.applyAdjustments,
        onCanvasWheel: session.coordinates.onCanvasWheel,
        focusAndShowChrome: session.coordinates.focusImageEditorAndShowChrome,
        scheduleChromeAnchorUpdate: session.coordinates.scheduleImageEditorChromeAnchorUpdate,
      },
      chrome: chrome.chromeCommands,
    },
  }
}
