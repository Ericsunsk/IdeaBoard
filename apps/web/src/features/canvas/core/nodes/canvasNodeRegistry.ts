import type {
  CanvasCreatableNodeKind,
  CanvasConnectionNodeKind,
  CanvasConnectionPort,
  CanvasExecutionOutputNodeKind,
} from '../../../board/ports/canvas'

/** Copy resolver injected by the caller (application/presentation own i18n). */
export type CanvasNodeCopyResolver = (labelKey: string) => string

export type ExecutionActionLabels = { ready: string; running: string }

type ExecutionActionLabelKeys = { readyKey: string; runningKey: string }

type CanvasNodeUiRegistryItem = {
  kind: CanvasConnectionNodeKind
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

export function getCanvasNodeExecutionActionLabels(
  nodeType: CanvasExecutionOutputNodeKind,
  translate: CanvasNodeCopyResolver,
): ExecutionActionLabels {
  const keys = getCanvasNodeUiRegistryItem(nodeType).primaryAction
  if (!keys) throw new Error(`Canvas node ${nodeType} has no execution action labels`)
  // Resolved read-time so locale copy stays live; the registry only owns keys.
  return { ready: translate(keys.readyKey), running: translate(keys.runningKey) }
}

export function formatCanvasNodePortLabel(
  port: CanvasConnectionPort,
  translate: CanvasNodeCopyResolver,
): string {
  if (port === 'image') return translate('canvas.node.image')
  if (port === 'prompt') return translate('canvas.node.prompt')
  return port
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
