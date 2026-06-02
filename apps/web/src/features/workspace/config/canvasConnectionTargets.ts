import { CANVAS_CONNECTION_NODE_TYPES, type CanvasConnectionPort } from '../types/canvasNodes'

export const CANVAS_CONNECTION_NODE_TYPE_WHITELIST = CANVAS_CONNECTION_NODE_TYPES

export const CANVAS_CONNECTION_PORT_WHITELIST: readonly CanvasConnectionPort[] = [
  'image',
  'prompt',
] as const

export const CANVAS_CONNECTION_PORT_DIRECTIONS = ['input', 'output'] as const

const CANVAS_CONNECTION_PORT_SET = new Set<string>(CANVAS_CONNECTION_PORT_WHITELIST)
const CANVAS_CONNECTION_PORT_DIRECTION_SET = new Set<string>(CANVAS_CONNECTION_PORT_DIRECTIONS)

export function isCanvasConnectionPort(value: unknown): value is CanvasConnectionPort {
  return typeof value === 'string' && CANVAS_CONNECTION_PORT_SET.has(value)
}

export function isCanvasPortDirection(value: unknown): value is 'input' | 'output' {
  return typeof value === 'string' && CANVAS_CONNECTION_PORT_DIRECTION_SET.has(value)
}
