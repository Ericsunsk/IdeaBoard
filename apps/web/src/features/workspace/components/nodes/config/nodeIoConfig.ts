// ═══════════════════════════════════════════════════════════════════════════
// MAINTENANCE CONSTRAINT: The `top` field on every IO slot template is the
// SOLE Y-coordinate source for Vue Flow <Handle> positioning.
//
// FlowNodeHandles.vue reads these tops via getNodeIoTopMap() for Handle positioning
// and applies them directly as CSS `top` values on Handles.  Do NOT re-
// introduce DOM-rect-based measurement (getBoundingClientRect) as a primary
// positioning source — it bakes the viewport transform into node-local
// coordinates and causes port drift at non-default zoom levels.
//
// When adding a new node type or editing an existing one:
//   - Set `top` on every slot that has a `direction` + `portName`.
//   - Match the `top` value to the port's visual center in the node body.
//   - If a port is conditional (e.g. Edit mask port shifts when imageB
//     is visible), use a resolver function.
// ═══════════════════════════════════════════════════════════════════════════

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
  direction?: NodeIoDirection
  portName?: string
  portKey?: string
  top?: Resolvable<T, number>
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
  {
    key: 'output',
    kind: 'prompt',
    direction: 'output',
    portName: 'prompt',
    portKey: 'prompt-output',
    top: 92,
  },
]

export const IMAGE_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    kind: 'image',
    direction: 'input',
    portName: 'image',
    portKey: 'image-input',
    top: 92,
  },
  {
    key: 'output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'image-output',
    top: 92,
  },
]

export const MASK_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    kind: 'image',
    direction: 'input',
    portName: 'image',
    portKey: 'mask-image-input',
    top: 92,
  },
  {
    key: 'output',
    kind: 'mask',
    direction: 'output',
    portName: 'mask',
    portKey: 'mask-output',
    top: 92,
  },
]

export const COLOR_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'image',
    portKey: 'color-image-input',
    top: 96,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
    direction: 'input',
    portName: 'mask',
    portKey: 'color-mask-input',
    top: 152,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    direction: 'input',
    portName: 'prompt',
    portKey: 'color-prompt-input',
    top: 208,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'color-output',
    top: 96,
  },
]

export const MODEL_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'image',
    portKey: 'model-image-input',
    top: 92,
  },
  {
    key: 'modelImage',
    label: 'Model',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'imageA',
    portKey: 'model-reference-image-input',
    top: 136,
  },
  {
    key: 'poseImage',
    label: 'Pose',
    kind: 'image',
    direction: 'input',
    portName: 'imageB',
    portKey: 'model-pose-image-input',
    top: 180,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    direction: 'input',
    portName: 'prompt',
    portKey: 'model-prompt-input',
    top: 224,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'model-output',
    top: 92,
  },
]

export const VIEWPOINT_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'image',
    label: 'Image',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'image',
    portKey: 'viewpoint-image-input',
    top: 112,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    direction: 'input',
    portName: 'prompt',
    portKey: 'viewpoint-prompt-input',
    top: 168,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'viewpoint-output',
    top: 112,
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
    direction: 'input',
    portName: 'image',
    portKey: 'edit-image-input',
    top: 92,
  },
  {
    key: 'imageB',
    label: (ctx) => ctx.sourceLabel,
    kind: 'image',
    required: (ctx) => ctx.supportsSourceImage,
    direction: 'input',
    portName: 'imageB',
    portKey: 'edit-image-b-input',
    top: 136,
    visible: (ctx) => ctx.supportsSourceImage,
  },
  {
    key: 'mask',
    label: (ctx) => ctx.maskLabel,
    kind: 'mask',
    required: (ctx) => ctx.requiresMask,
    direction: 'input',
    portName: 'mask',
    portKey: 'edit-mask-input',
    top: (ctx) => (ctx.supportsSourceImage ? 180 : 136),
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    required: (ctx) => ctx.supportsPrompt,
    direction: 'input',
    portName: 'prompt',
    portKey: 'edit-prompt-input',
    top: (ctx) => (ctx.supportsSourceImage ? 224 : 180),
    visible: (ctx) => ctx.supportsPrompt,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'edit-output',
    top: 92,
  },
]

export const FUSION_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'imageA',
    label: 'Image A',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'imageA',
    portKey: 'fusion-image-a',
    top: 92,
  },
  {
    key: 'imageB',
    label: 'Image B',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'imageB',
    portKey: 'fusion-image-b',
    top: 136,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    direction: 'input',
    portName: 'prompt',
    portKey: 'fusion-prompt',
    top: 180,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'fusion-output',
    top: 92,
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
    direction: 'input',
    portName: 'image',
    portKey: 'enhance-image-input',
    top: 92,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
    direction: 'input',
    portName: 'mask',
    portKey: 'enhance-mask-input',
    top: 136,
    visible: (ctx) => ctx.supportsMask,
  },
  {
    key: 'prompt',
    label: 'Prompt',
    kind: 'prompt',
    direction: 'input',
    portName: 'prompt',
    portKey: 'enhance-prompt-input',
    top: 136,
    visible: (ctx) => ctx.supportsPrompt,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'enhance-output',
    top: 92,
  },
]

export const WEAR_NODE_IO_SLOTS: readonly NodeIoSlotTemplate<null>[] = [
  {
    key: 'imageA',
    label: 'Body',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'imageA',
    portKey: 'wear-image-a',
    top: 92,
  },
  {
    key: 'imageB',
    label: 'Texture',
    kind: 'image',
    required: true,
    direction: 'input',
    portName: 'imageB',
    portKey: 'wear-image-b',
    top: 136,
  },
  {
    key: 'mask',
    label: 'Mask',
    kind: 'mask',
    direction: 'input',
    portName: 'mask',
    portKey: 'wear-mask-input',
    top: 180,
  },
  {
    key: 'output',
    label: 'Output',
    kind: 'image',
    direction: 'output',
    portName: 'image',
    portKey: 'wear-output',
    top: 92,
  },
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

    if (slot.direction && slot.portName) {
      ports.push({
        key: slot.portKey ?? slot.key,
        direction: slot.direction,
        kind: slot.kind,
        name: slot.portName,
        top: slot.top == null ? undefined : resolveValue(slot.top, context),
        connected,
        pending,
        visible,
      })
    }
  }

  return { connectionChips, ports }
}

export type { NodeConnectionChipItem, NodeIoSlotTemplate, NodePortItem }
