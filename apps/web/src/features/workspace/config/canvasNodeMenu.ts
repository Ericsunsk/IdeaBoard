export type CanvasNodeCreateActionKey =
  | 'createImageNode'
  | 'createPromptNode'
  | 'createViewpointNode'
  | 'createFusionNode'

export interface CanvasNodeMenuItem {
  key: string
  label: string
  actionKey: CanvasNodeCreateActionKey
  group?: string
  order?: number
}

export const CANVAS_NODE_MENU_ITEMS: readonly CanvasNodeMenuItem[] = [
  { key: 'image', label: '+ Image 节点', actionKey: 'createImageNode', group: 'source', order: 10 },
  {
    key: 'prompt',
    label: '+ Prompt 节点',
    actionKey: 'createPromptNode',
    group: 'source',
    order: 20,
  },
  {
    key: 'fusion',
    label: '+ Fusion 节点',
    actionKey: 'createFusionNode',
    group: 'process',
    order: 60,
  },
  {
    key: 'viewpoint',
    label: '+ Display 节点',
    actionKey: 'createViewpointNode',
    group: 'process',
    order: 80,
  },
] as const

export const CANVAS_NODE_MENU_BY_ACTION = Object.freeze(
  CANVAS_NODE_MENU_ITEMS.reduce<Record<CanvasNodeCreateActionKey, CanvasNodeMenuItem>>(
    (acc, item) => {
      acc[item.actionKey] = item
      return acc
    },
    {} as Record<CanvasNodeCreateActionKey, CanvasNodeMenuItem>,
  ),
)
