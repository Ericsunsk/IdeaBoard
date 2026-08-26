import type {
  CanvasCreatableNodeKind,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeType,
} from '../../canvas-board/domain'
import { i18n } from '../../../shared/i18n'

export type ExecutionActionLabels = { ready: string; running: string }

type ExecutionActionLabelKeys = { readyKey: string; runningKey: string }

type CanvasNodeUiRegistryItem = {
  kind: CanvasConnectionNodeType
  defaultTitle: string
  primaryAction?: ExecutionActionLabelKeys
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
    primaryAction: {
      readyKey: 'canvas.workflow.generate',
      runningKey: 'canvas.node.processing',
    },
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
  const keys = getCanvasNodeUiRegistryItem(nodeType).primaryAction
  if (!keys) throw new Error(`Canvas node ${nodeType} has no execution action labels`)
  // Resolved read-time so locale copy stays live; the registry only owns keys.
  return { ready: i18n.global.t(keys.readyKey), running: i18n.global.t(keys.runningKey) }
}

export function formatCanvasNodePortLabel(port: CanvasConnectionPort): string {
  if (port === 'image') return 'Image'
  if (port === 'prompt') return 'Prompt'
  return port
}

/** Menu copy resolved read-time; callers render through their own t(). */
export function getCanvasNodeKindMenuItems(): readonly { key: CanvasCreatableNodeKind; label: string }[] {
  return CANVAS_NODE_UI_REGISTRY_LIST.map((item) => ({
    key: item.kind as CanvasCreatableNodeKind,
    label: item.defaultTitle,
  }))
}
