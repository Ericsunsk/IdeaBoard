import type { EditNodeItem } from '../../../types/canvasNodes'
import {
  editModeRequiresMask,
  editModeSupportsPrompt,
  editModeSupportsSourceImage,
  getEditNodePortLabel,
} from '../../../config/canvasNodeConnectionContract'

export type NodeIoKind = 'image' | 'mask' | 'prompt'
export type NodeIoDirection = 'input' | 'output'

type Resolvable<T, V> = V | ((context: T) => V)

type NodeIoSlotTemplate<T> = {
  key: string
  kind: NodeIoKind
  label?: Resolvable<T, string>
  required?: Resolvable<T, boolean>
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

function resolveValue<T, V>(value: Resolvable<T, V>, context: T): V {
  return typeof value === 'function' ? (value as (ctx: T) => V)(context) : value
}

export const PROMPT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    kind: 'image',
    label: 'Image',
  },
  {
    key: 'output',
    kind: 'image',
  },
  {
    key: 'promptOutput',
    label: 'Prompt',
    kind: 'prompt',
  },
]

export const IMAGE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    kind: 'image',
  },
  {
    key: 'output',
    kind: 'image',
  },
]

export const MASK_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    kind: 'image',
  },
  {
    key: 'output',
    kind: 'mask',
  },
]

export const COLOR_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export const MODEL_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
  },
  {
    key: 'modelImage',
    label: 'Model',
    kind: 'image',
    required: true,
  },
  {
    key: 'poseImage',
    label: 'Pose',
    kind: 'image',
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export const VIEWPOINT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export type EditNodeIoContext = {
  imageLabel: string
  sourceLabel: string
  maskLabel: string
  supportsPrompt: boolean
  supportsSourceImage: boolean
  requiresMask: boolean
}

export function buildEditNodeIoContext(node: Pick<EditNodeItem, 'mode'>): EditNodeIoContext {
  return {
    imageLabel: getEditNodePortLabel(node.mode, 'image'),
    sourceLabel: getEditNodePortLabel(node.mode, 'imageB'),
    maskLabel: getEditNodePortLabel(node.mode, 'mask'),
    supportsPrompt: editModeSupportsPrompt(node.mode),
    supportsSourceImage: editModeSupportsSourceImage(node.mode),
    requiresMask: editModeRequiresMask(node.mode),
  }
}

export const EDIT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<EditNodeIoContext>[] = [
  {
    key: 'image',
    label: (ctx) => ctx.imageLabel,
    kind: 'image',
    required: true,
  },
  {
    key: 'imageB',
    label: (ctx) => ctx.sourceLabel,
    kind: 'image',
    required: (ctx) => ctx.supportsSourceImage,
    visible: (ctx) => ctx.supportsSourceImage,
  },
  {
    key: 'mask',
    label: (ctx) => ctx.maskLabel,
    kind: 'mask',
    required: (ctx) => ctx.requiresMask,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    required: (ctx) => ctx.supportsPrompt,
    visible: (ctx) => ctx.supportsPrompt,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export const FUSION_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'imageA',
    label: 'Image A',
    kind: 'image',
    required: true,
  },
  {
    key: 'imageB',
    label: 'Image B',
    kind: 'image',
    required: true,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export type EnhanceNodeIoContext = {
  supportsPrompt: boolean
  supportsMask: boolean
}

export const ENHANCE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<EnhanceNodeIoContext>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
    visible: (ctx) => ctx.supportsMask,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    visible: (ctx) => ctx.supportsPrompt,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export const WEAR_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'imageA',
    label: 'Body',
    kind: 'image',
    required: true,
  },
  {
    key: 'imageB',
    label: 'Texture',
    kind: 'image',
    required: true,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
  },
]

export function buildNodeIoFromSlots<T>(
  slots: readonly NodeIoSlotTemplate<T>[],
  context: T,
  state: NodeIoBuildState,
): { connectionChips: NodeConnectionChipItem[] } {
  const errorSet = new Set(state.errorPortKeys ?? [])
  const connectedMap = state.connected ?? {}
  const pendingMap = state.pending ?? {}

  const connectionChips: NodeConnectionChipItem[] = []

  for (const slot of slots) {
    const connected = Boolean(connectedMap[slot.key])
    const pending = Boolean(pendingMap[slot.key])
    const error = errorSet.has(slot.key)
    const required = slot.required == null ? undefined : resolveValue(slot.required, context)
    const visible = slot.visible == null ? undefined : resolveValue(slot.visible, context)

    if (slot.label != null) {
      connectionChips.push({
        key: slot.key,
        label: resolveValue(slot.label, context),
        kind: slot.kind,
        required,
        connected,
        pending,
        error,
        visible,
      })
    }
  }

  return { connectionChips }
}

export type { NodeConnectionChipItem, NodeIoSlotTemplate }
