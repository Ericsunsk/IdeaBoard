import type { Ref } from 'vue'
import type {
  CanvasImageItem,
  CanvasPoint,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  PromptNodeItem,
  GenerateNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

export interface MessageApiLike {
  success(content: string): unknown
}

export interface UseCanvasEditorOptions {
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  mannequinNodes: Ref<MannequinNodeItem[]>
  viewpointNodes: Ref<ViewpointNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  editNodes: Ref<EditNodeItem[]>
  fusionNodes: Ref<FusionNodeItem[]>
  enhanceNodes: Ref<EnhanceNodeItem[]>
  tryOnNodes: Ref<TryOnNodeItem[]>
  faceSwapNodes: Ref<FaceSwapNodeItem[]>
  paletteNodes: Ref<PaletteNodeItem[]>
  generateNodes: Ref<GenerateNodeItem[]>
  activeTool: Ref<string>
  showStyleStrengthControl: Ref<boolean>
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  canvasLayoutRef: Ref<HTMLElement | null>
  isBoardMenuOpen: () => boolean
  setBoardMenuOpen: (visible: boolean) => void
  isBoardRenaming: () => boolean
  isLeaveConfirmVisible: () => boolean
  activeColorPopoverNodeId: () => number | null
  confirmBoardRename: () => void
  resolveLeaveConfirm: (decision: 'save' | 'discard' | 'cancel') => void
  closeColorPopover: () => void
  updateZoom: (delta: number, anchor?: { clientX: number; clientY: number }) => void
  getCanvasPointFromClient: (clientX: number, clientY: number) => CanvasPoint
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  uploadImage: (file: File) => Promise<{ code?: number; data?: { url?: string } }>
  importCanvasImageToImageNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMaskNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToModelNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMannequinNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToViewpointNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  deleteImageNode: (id: number) => void
  deletePromptNode: (id: number) => void
  deleteModelNode: (id: number) => void
  deleteMannequinNode: (id: number) => void
  deleteViewpointNode: (id: number) => void
  deleteMaskNode: (id: number) => void
  deleteColorNode: (id: number) => void
  deleteEditNode: (id: number) => void
  deleteFusionNode: (id: number) => void
  deleteEnhanceNode: (id: number) => void
  deleteTryOnNode: (id: number) => void
  deleteFaceSwapNode: (id: number) => void
  deletePaletteNode: (id: number) => void
  deleteGenerateNode: (id: number) => void
  hasSelectedConnection?: () => boolean
  clearSelectedConnection?: () => void
  deleteSelectedConnection?: () => boolean
  beginHistoryBatch: () => void
  endHistoryBatch: (commit?: boolean) => void
  undo: () => Promise<boolean> | boolean
  redo: () => Promise<boolean> | boolean
  message: MessageApiLike
  nodeImageDragMime: string
}
