import { ref } from 'vue'

type ClearActiveNodePanelsOptions = {
  keepImageAction?: boolean
  keepImagePrompt?: boolean
  keepFusionPrompt?: boolean
}

export function useFlowActiveNodePanels() {
  const activeImageActionNodeId = ref<string | null>(null)
  const activeImagePromptNodeId = ref<string | null>(null)
  const activeFusionPromptNodeId = ref<string | null>(null)

  function closeActiveImagePromptComposer() {
    activeImagePromptNodeId.value = null
  }

  function clearActiveNodePanels(options?: ClearActiveNodePanelsOptions) {
    if (!options?.keepImageAction) activeImageActionNodeId.value = null
    if (!options?.keepImagePrompt) activeImagePromptNodeId.value = null
    if (!options?.keepFusionPrompt) activeFusionPromptNodeId.value = null
  }

  return {
    activeImageActionNodeId,
    activeImagePromptNodeId,
    activeFusionPromptNodeId,
    closeActiveImagePromptComposer,
    clearActiveNodePanels,
  }
}
