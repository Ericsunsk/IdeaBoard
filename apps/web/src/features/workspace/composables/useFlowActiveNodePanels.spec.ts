import { describe, expect, it } from 'vitest'
import { ACTIVE_NODE_PANEL_KIND, useFlowActiveNodePanels } from './useFlowActiveNodePanels'

describe('useFlowActiveNodePanels', () => {
  it('keeps node panels mutually exclusive through one active panel state', () => {
    const panels = useFlowActiveNodePanels()

    expect(panels.activeNodePanel.value).toBeNull()

    panels.openActiveNodePanel({ kind: ACTIVE_NODE_PANEL_KIND.imageTools, nodeId: 'image-1' })

    expect(panels.isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, 'image-1')).toBe(true)
    expect(panels.isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, 'image-1')).toBe(false)
    expect(panels.getActiveNodePanelNodeId(ACTIVE_NODE_PANEL_KIND.imageTools)).toBe('image-1')

    panels.openActiveNodePanel({ kind: ACTIVE_NODE_PANEL_KIND.fusionPrompt, nodeId: 'fusion-1' })

    expect(panels.isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.imageTools, 'image-1')).toBe(false)
    expect(panels.isActiveNodePanel(ACTIVE_NODE_PANEL_KIND.fusionPrompt, 'fusion-1')).toBe(true)
    expect(panels.getActiveNodePanelNodeId(ACTIVE_NODE_PANEL_KIND.imageTools)).toBeNull()
    expect(panels.getActiveNodePanelNodeId(ACTIVE_NODE_PANEL_KIND.fusionPrompt)).toBe('fusion-1')

    panels.clearActiveNodePanel()

    expect(panels.activeNodePanel.value).toBeNull()
  })
})
