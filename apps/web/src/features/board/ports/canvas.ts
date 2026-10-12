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
 * behavior and ownership stay in `board/core` and `board/session`.
 */

// Durable snapshot facts, pure validation, and media-ref extraction
// (extraction feeds backend refcount and GC lock). Canvas graph and workflow
// code also consumes these Board-owned pure document contracts; the explicit
// list keeps the port narrower than the full Board barrel.
export {
  FUSION_SLOT_LAYOUT,
  areFusionBlendPointsEqual,
  buildFusionDisplayPercentages,
  computeFusionBlendWeights,
  constrainFusionBlendPoint,
  getDefaultBlendPointForImageCount,
  getFusionLineAnchors,
  getFusionSlotAnchor,
  getFusionSlotAnchors,
  normalizeFusionBlendPoint,
  type FusionBlendPoint,
} from '../core/document/fusionBlend'
export {
  IMAGE_NODE_EMPTY_SIZE,
  resolveImageNodeDisplayRatio,
  resolveImageNodeFrameSize,
  resolveImageNodeLatestOutput,
  resolveImageNodeStageHeight,
} from '../core/document/imageNodeSizing'
export {
  InvalidBoardStateError,
  assertLatestBoardSnapshot,
  assertLatestBoardState,
} from '../core/document/latestBoardState'
export {
  CANVAS_NODE_REGISTRY_LIST,
  createTypedCanvasNodeSnapshot,
  getCanvasNodeImageInputLimit,
  getCanvasNodePortInputLimit,
  getCanvasNodePrimaryOutputPort,
  getCanvasNodeRequiredInputPorts,
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
  isCanvasConnectionNodeType,
  isCanvasExecutionOutputNodeType,
  isCanvasImageInputSourceNodeType,
  type CanvasConnectionContractNode,
  type CanvasCreatableNodeKind,
  type CanvasNodePortContract,
  type CanvasSnapshotNodeCollectionKey,
} from '../core/document/nodeRegistry'
export {
  IMAGE_OUTPUT_COUNT_PRESET_OPTIONS,
  createEmptyBoardSnapshot,
  requireImageNodeOutputCount,
  resolveWorkflowNodeOutputStatus,
  type BoardSnapshot,
  type BoardState,
  type CanvasConnection,
  type CanvasConnectionEndpoint,
  type CanvasConnectionNodeKind,
  type CanvasConnectionPort,
  type CanvasExecutionOutputNodeItem,
  type CanvasExecutionOutputNodeKind,
  type CanvasGroup,
  type CanvasGroupNodeId,
  type CanvasNodeItem,
  type CanvasNodeItemByKind,
  type CanvasPoint,
  type FusionNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
  type ImageRetouchRecipe,
  type ImageRetouchRecipeStep,
  type LibraryImportFact,
  type PromptNodeInputImage,
  type PromptNodeItem,
  type WorkflowNodeOutputStatus,
} from '../core/document/nodes'
export {
  PROMPT_ASPECT_RATIO_OPTIONS,
  normalizePromptAspectRatio,
  resolvePromptAspectRatioValue,
  type PromptAspectRatio,
} from '../core/document/promptAspectRatio'
export { rebuildBoardMediaRefs } from '../core/media/references'

// Local persistence and preview adapters.
export { loadBoardState, setBoardPreviewSnapshot, setBoardState } from '../session/persistence/state'
export { serializeBoardState } from '../session/persistence/boardStateSerialization'
export { BOARD_PREVIEW_RENDERER_VERSION } from '../session/preview/boardPreviewVersion'
export { renderBoardPreviewDataUrl } from '../session/preview/boardPreviewSerializer'

// Catalog and revision-gated sync use cases.
export { getBoardById, normalizeBoardName, touchBoard } from '../session/catalog/boards'
export { useBoardRepository, type BoardRepository } from '../session/catalog/repository'
export {
  fetchBoardArchivedAt,
  isBoardArchivedError,
  isBoardSyncConflictError,
  markBoardArchivedLocally,
  syncBoardToServer,
  syncRestoreBoardToServer,
  type BoardSyncConflictKind,
} from '../session/sync/boardSync'
