export const EDIT_NODE_MODE = {
  partialEdit: 'partial-edit',
  bgSwap: 'bg-swap',
  faceSwap: 'face-swap',
} as const

export type EditNodeMode = (typeof EDIT_NODE_MODE)[keyof typeof EDIT_NODE_MODE]

export const EDIT_NODE_MODE_LIST: readonly EditNodeMode[] = [
  EDIT_NODE_MODE.partialEdit,
  EDIT_NODE_MODE.bgSwap,
  EDIT_NODE_MODE.faceSwap,
]

export function isEditNodeMode(value: unknown): value is EditNodeMode {
  return typeof value === 'string' && EDIT_NODE_MODE_LIST.includes(value as EditNodeMode)
}

export const EDIT_NODE_MODE_OPTIONS = [
  { value: EDIT_NODE_MODE.partialEdit, label: 'Detail' },
  { value: EDIT_NODE_MODE.bgSwap, label: 'Background' },
  { value: EDIT_NODE_MODE.faceSwap, label: 'Face' },
] as const
