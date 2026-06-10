import {
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_FUSION_BLEND_POINT,
  getDefaultCanvasNodeTitle,
} from '../types/canvasNodes'
import type {
  CanvasPoint,
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeItem,
} from '../types/canvasNodes'

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function createPromptNodeAt(point: CanvasPoint): PromptNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'prompt',
    x: point.x,
    y: point.y,
    width: 280,
    title: getDefaultCanvasNodeTitle('prompt'),
    text: '',
    outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    inputImages: [],
  }
}

export function createImageNodeAt(point: CanvasPoint): ImageNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'image-node',
    x: point.x,
    y: point.y,
    width: 280,
    title: getDefaultCanvasNodeTitle('image'),
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createFusionNodeAt(point: CanvasPoint): FusionNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'fusion',
    x: point.x,
    y: point.y,
    width: 286,
    title: getDefaultCanvasNodeTitle('fusion'),
    prompt: '',
    outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    blendPoint: { ...DEFAULT_FUSION_BLEND_POINT },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}
