import { computed, provide, readonly } from 'vue'
import {
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  type CanvasImageAdjustmentFinalApplyContext,
} from '../image-editor/runtime/imageAdjustmentFinalApplyContext'
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
        finalApplyProgress: session.imageAdjustmentFinalApplyProgress,
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
        onCanvasWheel: session.viewport.onCanvasWheel,
        focusAndShowChrome: session.viewport.focusImageEditorAndShowChrome,
        scheduleChromeAnchorUpdate: session.viewport.scheduleImageEditorChromeAnchorUpdate,
      },
      prompt: {
        runPromptAction: commands.runPromptAction,
      },
      chrome: chrome.chromeCommands,
    },
  }
}
