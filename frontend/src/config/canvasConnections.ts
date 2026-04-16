import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
} from '../types/canvasNodes'

export type CanvasConnectionTone = 'image' | 'mask' | 'prompt'

export interface CanvasConnectionEndpointLike {
  nodeType: CanvasConnectionNodeType
  nodeId: number
  port: CanvasConnectionPort
}

export interface CanvasConnectionRule {
  fromNodeType: CanvasConnectionNodeType
  fromPort: CanvasConnectionPort
  toNodeType: CanvasConnectionNodeType
  toPort: CanvasConnectionPort
}

export const CANVAS_CONNECTION_RULES: readonly CanvasConnectionRule[] = [
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'mask', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'color', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'model', toPort: 'image' },
  { fromNodeType: 'model', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'color', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'mask', fromPort: 'mask', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'mask', fromPort: 'mask', toNodeType: 'color', toPort: 'mask' },
  { fromNodeType: 'mask', fromPort: 'mask', toNodeType: 'edit', toPort: 'mask' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'color', toPort: 'prompt' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'model', toPort: 'prompt' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'edit', toPort: 'prompt' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'edit', toPort: 'image' },
  { fromNodeType: 'edit', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'fusion', toPort: 'imageA' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'fusion', toPort: 'imageB' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'fusion', toPort: 'prompt' },
  { fromNodeType: 'fusion', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'enhance', toPort: 'image' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'enhance', toPort: 'prompt' },
  { fromNodeType: 'enhance', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'tryon', toPort: 'imageA' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'tryon', toPort: 'imageB' },
  { fromNodeType: 'tryon', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'palette', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'palette', toPort: 'reference' },
  { fromNodeType: 'palette', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
  { fromNodeType: 'image', fromPort: 'image', toNodeType: 'generate', toPort: 'image' },
  { fromNodeType: 'prompt', fromPort: 'prompt', toNodeType: 'generate', toPort: 'prompt' },
  { fromNodeType: 'generate', fromPort: 'image', toNodeType: 'image', toPort: 'image' },
] as const

function buildConnectionRuleKey(rule: CanvasConnectionRule): string {
  return `${rule.fromNodeType}:${rule.fromPort}->${rule.toNodeType}:${rule.toPort}`
}

const CANVAS_CONNECTION_RULE_KEY_SET = new Set<string>(
  CANVAS_CONNECTION_RULES.map(buildConnectionRuleKey),
)

export function resolveCanvasConnectionTone(port: CanvasConnectionPort): CanvasConnectionTone {
  if (port === 'mask') return 'mask'
  if (port === 'prompt') return 'prompt'
  return 'image'
}

export function isCanvasConnectionAllowed(
  from: CanvasConnectionEndpointLike,
  to: CanvasConnectionEndpointLike,
): boolean {
  if (from.nodeType === to.nodeType && from.nodeId === to.nodeId) return false
  return CANVAS_CONNECTION_RULE_KEY_SET.has(
    buildConnectionRuleKey({
      fromNodeType: from.nodeType,
      fromPort: from.port,
      toNodeType: to.nodeType,
      toPort: to.port,
    }),
  )
}
