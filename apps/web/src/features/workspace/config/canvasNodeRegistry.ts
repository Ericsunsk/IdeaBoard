import type {
  CanvasCreatableNodeKind,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeType,
} from '../../canvas-board/domain'

interface CanvasNodeMenuItem {
  key: CanvasCreatableNodeKind
  label: string
}

type ExecutionActionLabels = { ready: string; running: string }

type CanvasNodeUiRegistryItem = {
  kind: CanvasConnectionNodeType
  defaultTitle: string
  primaryAction?: ExecutionActionLabels
}

const CANVAS_NODE_UI_REGISTRY = {
  image: {
    kind: 'image',
    defaultTitle: 'Image',
  },
  prompt: {
    kind: 'prompt',
    defaultTitle: 'Prompt',
  },
  fusion: {
    kind: 'fusion',
    defaultTitle: 'Fusion',
    primaryAction: { ready: '融合', running: '融合中...' },
  },
} as const satisfies Record<CanvasConnectionNodeType, CanvasNodeUiRegistryItem>

const CANVAS_NODE_UI_REGISTRY_LIST = Object.values(CANVAS_NODE_UI_REGISTRY)

function getCanvasNodeUiRegistryItem(nodeType: CanvasConnectionNodeType): CanvasNodeUiRegistryItem {
  return CANVAS_NODE_UI_REGISTRY[nodeType]
}

export function getDefaultCanvasNodeTitle(nodeType: CanvasConnectionNodeType): string {
  return getCanvasNodeUiRegistryItem(nodeType).defaultTitle
}

export function getCanvasNodeExecutionActionLabels(
  nodeType: CanvasExecutionOutputNodeType,
): ExecutionActionLabels {
  const labels = getCanvasNodeUiRegistryItem(nodeType).primaryAction
  if (!labels) throw new Error(`Canvas node ${nodeType} has no execution action labels`)
  return labels
}

export function formatCanvasNodePortLabel(port: CanvasConnectionPort): string {
  if (port === 'image') return 'Image'
  if (port === 'prompt') return 'Prompt'
  return port
}

export const CANVAS_NODE_MENU_ITEMS = CANVAS_NODE_UI_REGISTRY_LIST.map(
  (item) => ({
    key: item.kind,
    label: `+ ${item.defaultTitle} 节点`,
  }),
) as readonly CanvasNodeMenuItem[]
