/**
 * Pure Board document contracts for Canvas graph and workflow callers.
 * Browser persistence, catalog, and synchronization live in canvasSession.
 */

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
