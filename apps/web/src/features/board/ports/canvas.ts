/**
 * BoardCanvas port — the narrow Board surface for Canvas callers (issue #861).
 *
 * Canvas `board-session` orchestrates one open Board on top of this port:
 * snapshot facts, local persistence, revision-gated sync, preview
 * rendering, and media-ref extraction. It must NOT reach the Board barrel
 * (`features/board` index: catalog gallery presentation, collaboration
 * workflows, server hydrate facades) directly — every Canvas-needed Board
 * capability is curated here so the coupling stays explicit and minimal.
 *
 * This port re-exports Board-owned implementations without copying them;
 * behavior and ownership stay in `board/domain`, `board/application`, and
 * `board/infrastructure`.
 */

// Durable snapshot facts, pure validation, and media-ref extraction
// (extraction feeds backend refcount and GC lock). Canvas graph and workflow
// code also consumes these Board-owned pure document contracts; the explicit
// list keeps the port narrower than the full Board barrel.
export {
  CANVAS_NODE_REGISTRY_LIST,
  FUSION_SLOT_LAYOUT,
  IMAGE_NODE_EMPTY_SIZE,
  IMAGE_OUTPUT_COUNT_PRESET_OPTIONS,
  InvalidBoardStateError,
  PROMPT_ASPECT_RATIO_OPTIONS,
  areFusionBlendPointsEqual,
  assertLatestBoardSnapshot,
  assertLatestBoardState,
  buildFusionDisplayPercentages,
  computeFusionBlendWeights,
  constrainFusionBlendPoint,
  createEmptyBoardSnapshot,
  createTypedCanvasNodeSnapshot,
  getCanvasNodeImageInputLimit,
  getCanvasNodePortInputLimit,
  getCanvasNodePrimaryOutputPort,
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  getDefaultBlendPointForImageCount,
  getFusionLineAnchors,
  getFusionSlotAnchor,
  getFusionSlotAnchors,
  isCanvasConnectionNodeType,
  isCanvasExecutionOutputNodeType,
  isCanvasImageInputSourceNodeType,
  normalizeFusionBlendPoint,
  normalizePromptAspectRatio,
  rebuildBoardMediaRefs,
  requireImageNodeOutputCount,
  resolveImageNodeDisplayRatio,
  resolveImageNodeFrameSize,
  resolveImageNodeLatestOutput,
  resolveImageNodeStageHeight,
  resolvePromptAspectRatioValue,
  resolveWorkflowNodeOutputStatus,
  type BoardSnapshot,
  type BoardState,
  type CanvasConnection,
  type CanvasConnectionContractNode,
  type CanvasConnectionEndpoint,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasCreatableNodeKind,
  type CanvasExecutionOutputNodeItem,
  type CanvasExecutionOutputNodeType,
  type CanvasGroup,
  type CanvasGroupNodeId,
  type CanvasNodeItem,
  type CanvasNodeItemByKind,
  type CanvasNodePortContract,
  type CanvasPoint,
  type CanvasSnapshotNodeCollectionKey,
  type FusionBlendPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
  type ImageRetouchRecipe,
  type ImageRetouchRecipeStep,
  type ImageRetouchStack,
  type LibraryImportFact,
  type PromptAspectRatio,
  type PromptNodeInputImage,
  type PromptNodeItem,
  type WorkflowNodeOutputStatus,
} from '../domain'

// Local persistence and preview adapters.
export {
  loadBoardState,
  serializeBoardState,
  setBoardPreviewSnapshot,
  setBoardState,
} from '../infrastructure/persistence'
export {
  BOARD_PREVIEW_RENDERER_VERSION,
  renderBoardPreviewDataUrl,
} from '../infrastructure/preview'

// Catalog and revision-gated sync use cases.
export {
  getBoardById,
  normalizeBoardName,
  touchBoard,
  useBoardRepository,
  type BoardRepository,
} from '../application/catalog'
export {
  fetchBoardArchivedAt,
  isBoardArchivedError,
  isBoardNameConflictError,
  markBoardArchivedLocally,
  syncBoardToServer,
  syncRestoreBoardToServer,
} from '../application/sync'
