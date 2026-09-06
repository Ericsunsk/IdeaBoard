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
  defaultTitleKey: string
  primaryAction?: ExecutionActionLabelKeys
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
  return i18n.global.t(getCanvasNodeUiRegistryItem(nodeType).defaultTitleKey)
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
  if (port === 'image') return i18n.global.t('canvas.node.image')
  if (port === 'prompt') return i18n.global.t('canvas.node.prompt')
  return port
}

/** Menu copy resolved read-time; callers render through their own t(). */
export function getCanvasNodeKindMenuItems(): readonly {
  key: CanvasCreatableNodeKind
  label: string
}[] {
  return CANVAS_NODE_UI_REGISTRY_LIST.map((item) => ({
    key: item.kind as CanvasCreatableNodeKind,
    label: i18n.global.t(item.defaultTitleKey),
  }))
}
