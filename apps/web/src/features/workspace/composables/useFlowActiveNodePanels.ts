import { ref } from 'vue'

export const ACTIVE_NODE_PANEL_KIND = {
  imageTools: 'image-tools',
  fusionPrompt: 'fusion-prompt',
} as const

export type ActiveNodePanel =
  | { kind: typeof ACTIVE_NODE_PANEL_KIND.imageTools; nodeId: string }
  | { kind: typeof ACTIVE_NODE_PANEL_KIND.fusionPrompt; nodeId: string }

export type ActiveNodePanelKind = ActiveNodePanel['kind']

export function useFlowActiveNodePanels() {
  const activeNodePanel = ref<ActiveNodePanel | null>(null)

  function openActiveNodePanel(panel: ActiveNodePanel) {
    activeNodePanel.value = panel
  }

  function clearActiveNodePanel() {
    activeNodePanel.value = null
  }

  function isActiveNodePanel(kind: ActiveNodePanelKind, nodeId: string) {
    return activeNodePanel.value?.kind === kind && activeNodePanel.value.nodeId === nodeId
  }

  function getActiveNodePanelNodeId(kind: ActiveNodePanelKind): string | null {
    return activeNodePanel.value?.kind === kind ? activeNodePanel.value.nodeId : null
  }

  return {
    activeNodePanel,
    openActiveNodePanel,
    clearActiveNodePanel,
    isActiveNodePanel,
    getActiveNodePanelNodeId,
  }
}
