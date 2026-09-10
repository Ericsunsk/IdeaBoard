import { toRaw } from 'vue'
import type { ImageRetouchStack } from '../../../board/domain'
import type { RetouchEffectLayer, RetouchLayer } from '../types'

const STACK_VERSION = 1 as const

function isEffectLayer(layer: RetouchLayer): layer is RetouchEffectLayer {
  return layer.type === 'effect'
}

/**
 * Session layers are Vue reactive proxies, which structuredClone rejects.
 * toRaw unwraps them; params are plain domain value objects afterwards.
 */
function cloneParams(params: unknown): unknown {
  return structuredClone(toRaw(params) as object)
}

/** Export the session's adjustment layers to a durable board fact. */
export function serializeRetouchStack(
  layers: readonly RetouchLayer[],
  selectedLayerId: string,
): ImageRetouchStack {
  return {
    version: STACK_VERSION,
    layers: layers.filter(isEffectLayer).map((layer) => ({
      id: layer.id,
      kind: layer.kind,
      name: layer.name,
      visible: layer.visible,
      params: cloneParams(layer.params),
    })),
    selectedLayerId,
  }
}

/** Rebuild session layers from a persisted stack; returns null when absent/invalid. */
export function deserializeRetouchStack(
  stack: ImageRetouchStack | null | undefined,
): { layers: RetouchEffectLayer[]; selectedLayerId: string } | null {
  if (!stack || stack.version !== STACK_VERSION || !Array.isArray(stack.layers)) return null
  const layers: RetouchEffectLayer[] = []
  for (const item of stack.layers) {
    const layer = toEffectLayer(item)
    if (layer) layers.push(layer)
  }
  const selected =
    typeof stack.selectedLayerId === 'string' &&
    layers.some((layer) => layer.id === stack.selectedLayerId)
      ? stack.selectedLayerId
      : (layers[0]?.id ?? '')
  return { layers, selectedLayerId: selected }
}

function toEffectLayer(item: unknown): RetouchEffectLayer | null {
  if (!item || typeof item !== 'object') return null
  const candidate = item as {
    id?: unknown
    kind?: unknown
    name?: unknown
    visible?: unknown
    params?: unknown
  }
  if (typeof candidate.id !== 'string' || !candidate.id) return null
  if (!isStackEffectKind(candidate.kind)) return null
  if (typeof candidate.params !== 'object' || candidate.params === null) return null
  return {
    id: candidate.id,
    type: 'effect',
    kind: candidate.kind,
    name: typeof candidate.name === 'string' ? candidate.name : candidate.kind,
    visible: candidate.visible !== false,
    params: cloneParams(candidate.params),
  } as RetouchEffectLayer
}

function isStackEffectKind(
  kind: unknown,
): kind is 'hsl' | 'threshold' | 'curves' | 'levels' | 'sharpen' {
  return (
    kind === 'hsl' ||
    kind === 'threshold' ||
    kind === 'curves' ||
    kind === 'levels' ||
    kind === 'sharpen'
  )
}
