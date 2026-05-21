import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'

export const CANVAS_CONNECTION_NODE_TYPE_WHITELIST: readonly CanvasConnectionNodeType[] = [
  'image',
  'prompt',
  'mask',
  'model',
  'viewpoint',
  'edit',
  'fusion',
  'enhance',
  'wear',
] as const

export const CANVAS_CONNECTION_PORT_WHITELIST: readonly CanvasConnectionPort[] = [
  'image',
  'mask',
  'prompt',
  'imageA',
  'imageB',
] as const

export const CANVAS_CONNECTION_PORT_DIRECTIONS = ['input', 'output'] as const

const CANVAS_CONNECTION_NODE_TYPE_SET = new Set<string>(CANVAS_CONNECTION_NODE_TYPE_WHITELIST)
const CANVAS_CONNECTION_PORT_SET = new Set<string>(CANVAS_CONNECTION_PORT_WHITELIST)
const CANVAS_CONNECTION_PORT_DIRECTION_SET = new Set<string>(CANVAS_CONNECTION_PORT_DIRECTIONS)

export function isCanvasConnectionNodeType(value: unknown): value is CanvasConnectionNodeType {
  return typeof value === 'string' && CANVAS_CONNECTION_NODE_TYPE_SET.has(value)
}

export function isCanvasConnectionPort(value: unknown): value is CanvasConnectionPort {
  return typeof value === 'string' && CANVAS_CONNECTION_PORT_SET.has(value)
}

export function isCanvasPortDirection(value: unknown): value is 'input' | 'output' {
  return typeof value === 'string' && CANVAS_CONNECTION_PORT_DIRECTION_SET.has(value)
}
