import type { Ref } from 'vue'
import type {
  CanvasImageItem,
  CanvasPoint,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
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
  viewpointNodes: Ref<ViewpointNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  editNodes: Ref<EditNodeItem[]>
  fusionNodes: Ref<FusionNodeItem[]>
  enhanceNodes: Ref<EnhanceNodeItem[]>
  wearNodes: Ref<WearNodeItem[]>
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
  confirmBoardRename: () => Promise<boolean> | boolean
  saveBoard: () => Promise<boolean> | boolean
  resolveLeaveConfirm: (decision: 'save' | 'discard' | 'cancel') => void
  updateZoom: (delta: number, anchor?: { clientX: number; clientY: number }) => void
  getCanvasPointFromClient: (clientX: number, clientY: number) => CanvasPoint
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  uploadImage: (file: File) => Promise<{ code?: number; data?: { url?: string } }>
  resolveCanvasImageUploadMeta?: (url: string) => CanvasImageItem['uploadMeta'] | undefined
  importCanvasImageToImageNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMaskNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  deleteImageNode: (id: number) => void
  deletePromptNode: (id: number) => void
  deleteModelNode: (id: number) => void
  deleteViewpointNode: (id: number) => void
  deleteMaskNode: (id: number) => void
  deleteColorNode: (id: number) => void
  deleteEditNode: (id: number) => void
  deleteFusionNode: (id: number) => void
  deleteEnhanceNode: (id: number) => void
  deleteWearNode: (id: number) => void
  onDeleteCanvasImages?: (deletedItems: CanvasImageItem[]) => void
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
