export type NodeIoKind = 'image' | 'mask' | 'prompt'
export type NodeIoDirection = 'input' | 'output'

type Resolvable<T, V> = V | ((context: T) => V)

type NodeIoSlotTemplate<T> = {
  key: string
  kind: NodeIoKind
  label?: Resolvable<T, string>
  direction?: NodeIoDirection
  portName?: string
  portKey?: string
  top?: number
  required?: boolean
  visible?: Resolvable<T, boolean>
}

type NodeIoBuildState = {
  connected?: Partial<Record<string, boolean | undefined>>
  pending?: Partial<Record<string, boolean | undefined>>
  errorPortKeys?: readonly string[]
}

type NodeConnectionChipItem = {
  key: string
  label: string
  kind: NodeIoKind
  connected?: boolean
  pending?: boolean
  error?: boolean
  required?: boolean
  visible?: boolean
}

type NodePortItem = {
  key: string
  direction: NodeIoDirection
  kind: NodeIoKind
  name: string
  top?: number
  connected?: boolean
  pending?: boolean
  visible?: boolean
}

function resolveValue<T, V>(value: Resolvable<T, V>, context: T): V {
  return typeof value === 'function' ? (value as (ctx: T) => V)(context) : value
}

export const PROMPT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'output', kind: 'prompt', direction: 'output', portName: 'prompt', portKey: 'prompt-output' },
]

export const IMAGE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', kind: 'image', direction: 'input', portName: 'image', portKey: 'image-input' },
  { key: 'output', kind: 'image', direction: 'output', portName: 'image', portKey: 'image-output' },
]

export const MASK_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', kind: 'image', direction: 'input', portName: 'image', portKey: 'mask-image-input' },
  { key: 'output', kind: 'mask', direction: 'output', portName: 'mask', portKey: 'mask-output' },
]

export const COLOR_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'color-image-input', top: 96 },
  { key: 'mask', label: 'Mask', kind: 'mask', required: true, direction: 'input', portName: 'mask', portKey: 'color-mask-input', top: 152 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'color-prompt-input', top: 208 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'color-output', top: 96 },
]

export const MODEL_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'model-image-input', top: 92 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'model-prompt-input', top: 148 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'model-output', top: 92 },
]

export const MANNEQUIN_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'mannequin-image-input', top: 92 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'mannequin-prompt-input', top: 148 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'mannequin-output', top: 92 },
]

export const VIEWPOINT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'viewpoint-image-input', top: 92 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'viewpoint-prompt-input', top: 148 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'viewpoint-output', top: 92 },
]

export const EDIT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'edit-image-input', top: 92 },
  { key: 'mask', label: 'Mask', kind: 'mask', required: true, direction: 'input', portName: 'mask', portKey: 'edit-mask-input', top: 136 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'edit-prompt-input', top: 180 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'edit-output', top: 92 },
]

export const FUSION_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'imageA', label: 'Image A', kind: 'image', required: true, direction: 'input', portName: 'imageA', portKey: 'fusion-image-a', top: 92 },
  { key: 'imageB', label: 'Image B', kind: 'image', required: true, direction: 'input', portName: 'imageB', portKey: 'fusion-image-b', top: 136 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'fusion-prompt', top: 180 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'fusion-output', top: 92 },
]

export type EnhanceNodeIoContext = {
  supportsPrompt: boolean
  supportsMask: boolean
}

export const ENHANCE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<EnhanceNodeIoContext>[] = [
  { key: 'image', label: 'Image', kind: 'image', required: true, direction: 'input', portName: 'image', portKey: 'enhance-image-input', top: 92 },
  { key: 'mask', label: 'Mask', kind: 'mask', direction: 'input', portName: 'mask', portKey: 'enhance-mask-input', top: 136, visible: (ctx) => ctx.supportsMask },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', direction: 'input', portName: 'prompt', portKey: 'enhance-prompt-input', top: 136, visible: (ctx) => ctx.supportsPrompt },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'enhance-output', top: 92 },
]

export type TryOnNodeIoContext = {
  baseLabel: string
  layerLabel: string
}

export const TRYON_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<TryOnNodeIoContext>[] = [
  { key: 'imageA', label: (ctx) => ctx.baseLabel, kind: 'image', required: true, direction: 'input', portName: 'imageA', portKey: 'tryon-image-a', top: 92 },
  { key: 'imageB', label: (ctx) => ctx.layerLabel, kind: 'image', required: true, direction: 'input', portName: 'imageB', portKey: 'tryon-image-b', top: 136 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'tryon-output', top: 92 },
]

export const FACESWAP_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'imageA', label: 'Target', kind: 'image', required: true, direction: 'input', portName: 'imageA', portKey: 'faceswap-image-a', top: 92 },
  { key: 'imageB', label: 'Face', kind: 'image', required: true, direction: 'input', portName: 'imageB', portKey: 'faceswap-image-b', top: 136 },
  { key: 'mask', label: 'Mask', kind: 'mask', direction: 'input', portName: 'mask', portKey: 'faceswap-mask-input', top: 180 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'faceswap-output', top: 92 },
]

export const GENERATE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  { key: 'image', label: 'Image', kind: 'image', direction: 'input', portName: 'image', portKey: 'generate-image-input', top: 92 },
  { key: 'prompt', label: 'Prompt', kind: 'prompt', required: true, direction: 'input', portName: 'prompt', portKey: 'generate-prompt-input', top: 136 },
  { key: 'output', label: 'Output', kind: 'image', direction: 'output', portName: 'image', portKey: 'generate-output', top: 92 },
]

export function buildNodeIoFromSlots<T>(
  slots: readonly NodeIoSlotTemplate<T>[],
  context: T,
  state: NodeIoBuildState,
): { connectionChips: NodeConnectionChipItem[]; ports: NodePortItem[] } {
  const errorSet = new Set(state.errorPortKeys ?? [])
  const connectedMap = state.connected ?? {}
  const pendingMap = state.pending ?? {}

  const connectionChips: NodeConnectionChipItem[] = []
  const ports: NodePortItem[] = []

  for (const slot of slots) {
    const connected = Boolean(connectedMap[slot.key])
    const pending = Boolean(pendingMap[slot.key])
    const error = errorSet.has(slot.key)
    const visible = slot.visible == null ? undefined : resolveValue(slot.visible, context)

    if (slot.label != null) {
      connectionChips.push({
        key: slot.key,
        label: resolveValue(slot.label, context),
        kind: slot.kind,
        required: slot.required,
        connected,
        pending,
        error,
        visible,
      })
    }

    if (slot.direction && slot.portName) {
      ports.push({
        key: slot.portKey ?? slot.key,
        direction: slot.direction,
        kind: slot.kind,
        name: slot.portName,
        top: slot.top,
        connected,
        pending,
        visible,
      })
    }
  }

  return { connectionChips, ports }
}

export type { NodeConnectionChipItem, NodeIoSlotTemplate, NodePortItem }
