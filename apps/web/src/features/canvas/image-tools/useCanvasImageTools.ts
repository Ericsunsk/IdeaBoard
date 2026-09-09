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
  const nodeBindings = createImageToolNodeBindings(session, chrome.retouchStage)

  const isChromeHidden = computed(() => !!session.retouchNodeId.value)
  const isCanvasLocked = computed(
    () =>
      options.isCanvasInteractionLocked.value ||
      !!session.retouchNodeId.value ||
      session.applyScope.isApplying.value,
  )

  return {
    viewModel: {
      active: {
        nodeId: session.retouchNodeId,
        maskNodeId: session.inpaint.inpaintNodeId,
        mode: session.retouchMode,
      },
      locks: {
        isChromeHidden,
        isCanvasLocked,
      },
      chrome: chrome.chromeModel,
      retouch: {
        isApplying: session.applyScope.isApplying,
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
        apply: commands.applyRetouch,
        onCanvasWheel: session.coordinates.onCanvasWheel,
        focusAndShowChrome: session.coordinates.focusRetouchAndShowChrome,
        scheduleChromeAnchorUpdate: session.coordinates.scheduleRetouchChromeAnchorUpdate,
      },
      chrome: chrome.chromeCommands,
    },
  }
}
