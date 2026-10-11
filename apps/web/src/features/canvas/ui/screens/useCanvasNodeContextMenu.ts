import { computed, ref } from 'vue'
import type { CanvasCreatableNodeKind } from '../../../board/ports/canvas'
import { i18n } from '../../../../core/i18n'
import { getCanvasNodeKindMenuItems } from '../../core/nodes/canvasNodeRegistry.ts'

type CanvasNodeContextMenuSnapshot = {
  visible: boolean
  x: number
  y: number
  flowX: number
  flowY: number
}

export function useCanvasNodeContextMenu() {
  const nodeContextMenu = ref<CanvasNodeContextMenuSnapshot>({
    visible: false,
    x: 0,
    y: 0,
    flowX: 0,
    flowY: 0,
  })

  const canvasNodeMenuItems = computed(() =>
    getCanvasNodeKindMenuItems((key) => i18n.global.t(key)),
  )

  function close() {
    nodeContextMenu.value.visible = false
  }

  function open(snapshot: Omit<CanvasNodeContextMenuSnapshot, 'visible'>) {
    nodeContextMenu.value = { visible: true, ...snapshot }
  }

  function createNode(
    kind: CanvasCreatableNodeKind,
    createNodeAtPosition: (kind: CanvasCreatableNodeKind, flowX: number, flowY: number) => void,
  ) {
    const { flowX, flowY } = nodeContextMenu.value
    createNodeAtPosition(kind, flowX, flowY)
    close()
  }

  return {
    nodeContextMenu,
    canvasNodeMenuItems,
    close,
    open,
    createNode,
  }
}
