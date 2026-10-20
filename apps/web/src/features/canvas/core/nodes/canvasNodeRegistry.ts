import type { CanvasCreatableNodeKind, CanvasConnectionNodeKind } from '../../../board/ports/canvas'

/** Copy resolver injected by the caller (application/presentation own i18n). */
export type CanvasNodeCopyResolver = (labelKey: string) => string

type CanvasNodeUiRegistryItem = {
  kind: CanvasConnectionNodeKind
  defaultTitleKey: string
}

const CANVAS_NODE_UI_REGISTRY = {
  image: {
    kind: 'image',
    defaultTitleKey: 'canvas.node.image',
  },
  prompt: {
    kind: 'prompt',
    defaultTitleKey: 'canvas.node.prompt',
  },
  fusion: {
    kind: 'fusion',
    defaultTitleKey: 'canvas.node.fusion',
  },
} as const satisfies Record<CanvasConnectionNodeKind, CanvasNodeUiRegistryItem>

const CANVAS_NODE_UI_REGISTRY_LIST = Object.values(CANVAS_NODE_UI_REGISTRY)

function getCanvasNodeUiRegistryItem(nodeType: CanvasConnectionNodeKind): CanvasNodeUiRegistryItem {
  return CANVAS_NODE_UI_REGISTRY[nodeType]
}

export function getDefaultCanvasNodeTitle(
  nodeType: CanvasConnectionNodeKind,
  translate: CanvasNodeCopyResolver,
): string {
  return translate(getCanvasNodeUiRegistryItem(nodeType).defaultTitleKey)
}

/** Menu copy resolved read-time; callers render through their own t(). */
export function getCanvasNodeKindMenuItems(translate: CanvasNodeCopyResolver): readonly {
  key: CanvasCreatableNodeKind
  label: string
}[] {
  return CANVAS_NODE_UI_REGISTRY_LIST.map((item) => ({
    key: item.kind as CanvasCreatableNodeKind,
    label: translate(item.defaultTitleKey),
  }))
}
