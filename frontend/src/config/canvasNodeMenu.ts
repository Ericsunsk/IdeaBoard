export type CanvasNodeCreateActionKey =
  | 'createImageNode'
  | 'createPromptNode'
  | 'createModelNode'
  | 'createViewpointNode'
  | 'createMaskNode'
  | 'createEditNode'
  | 'createFusionNode'
  | 'createEnhanceNode'
  | 'createTryOnNode'
  | 'createColorNode'

export interface CanvasNodeMenuItem {
  key: string
  label: string
  actionKey: CanvasNodeCreateActionKey
  group?: string
  order?: number
}

export const CANVAS_NODE_MENU_ITEMS: readonly CanvasNodeMenuItem[] = [
  { key: 'image', label: '+ Image 节点', actionKey: 'createImageNode', group: 'source', order: 10 },
  { key: 'prompt', label: '+ Prompt 节点', actionKey: 'createPromptNode', group: 'source', order: 20 },
  { key: 'model', label: '+ 模特节点', actionKey: 'createModelNode', group: 'source', order: 30 },
  { key: 'mask', label: '+ Mask 节点', actionKey: 'createMaskNode', group: 'source', order: 40 },
  { key: 'edit', label: '+ Edit 节点', actionKey: 'createEditNode', group: 'process', order: 50 },
  { key: 'fusion', label: '+ Fusion 节点', actionKey: 'createFusionNode', group: 'process', order: 60 },
  { key: 'enhance', label: '+ Enhance 节点', actionKey: 'createEnhanceNode', group: 'process', order: 70 },
  { key: 'tryon', label: '+ Wear 节点', actionKey: 'createTryOnNode', group: 'process', order: 80 },
  { key: 'viewpoint', label: '+ Display 节点', actionKey: 'createViewpointNode', group: 'process', order: 90 },
  { key: 'color', label: '+ Color 节点', actionKey: 'createColorNode', group: 'process', order: 100 },
] as const

export const CANVAS_NODE_MENU_BY_ACTION = Object.freeze(
  CANVAS_NODE_MENU_ITEMS.reduce<Record<CanvasNodeCreateActionKey, CanvasNodeMenuItem>>((acc, item) => {
    acc[item.actionKey] = item
    return acc
  }, {} as Record<CanvasNodeCreateActionKey, CanvasNodeMenuItem>),
)
