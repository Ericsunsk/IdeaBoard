import { ref } from 'vue'
import type { FlowCreatableNodeKind } from './useFlowNodeMutations'
import { CANVAS_NODE_MENU_ITEMS } from '../config/canvasNodeMenu'

export function useFlowNodeContextMenu() {
  const nodeContextMenu = ref({ visible: false, x: 0, y: 0, flowX: 0, flowY: 0 })

  const canvasNodeMenuItems = CANVAS_NODE_MENU_ITEMS.map(item => ({
    key: item.key as FlowCreatableNodeKind,
    label: item.label.replace('+ ', '').replace(' 节点', ''),
  }))

  function closeNodeContextMenu() {
    nodeContextMenu.value.visible = false
  }

  function openNodeContextMenu(x: number, y: number, flowX: number, flowY: number) {
    nodeContextMenu.value = { visible: true, x, y, flowX, flowY }
  }

  return {
    nodeContextMenu,
    canvasNodeMenuItems,
    closeNodeContextMenu,
    openNodeContextMenu,
  }
}
