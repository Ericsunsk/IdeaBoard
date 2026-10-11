import { describe, expect, it } from 'vitest'
import type {
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../../../board'
import { getCanvasNodeImageInputLimit } from '../../../../board'
import { patchFlowSnapshotParam } from './flowSnapshotParamMutations'

function makeImageNode(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function makePromptNode(overrides: Partial<PromptNodeItem> = {}): PromptNodeItem {
  return {
    id: 2,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    text: '',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
    ...overrides,
  }
}

function makeFusionNode(overrides: Partial<FusionNodeItem> = {}): FusionNodeItem {
  return {
    id: 3,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 280,
    prompt: 'blend',
    outputCount: 1,
    aspectRatio: 'auto',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function makeInputImage(id: string): PromptNodeInputImage {
  return {
    id,
    sourceType: 'external',
    sourceId: id,
    url: `https://example.com/${id}.png`,
    ratio: 1,
    mediaAssetId: 301,
  }
}

describe('patchFlowSnapshotParam', () => {
  it('trims titles and ignores empty title patches', () => {
    const imageNode = makeImageNode({ title: 'Before' })

    expect(patchFlowSnapshotParam(imageNode, { key: 'title', value: '  After  ' })).toMatchObject({
      title: 'After',
    })
    expect(patchFlowSnapshotParam(imageNode, { key: 'title', value: '   ' })).toBe(imageNode)
  })

  it('rejects output counts outside the domain contract', () => {
    const promptNode = makePromptNode({ outputCount: 1 })
    expect(() => patchFlowSnapshotParam(promptNode, { key: 'outputCount', value: 99 })).toThrow(
      'requested output count must be an integer between 1 and 4',
    )
  })

  it('applies prompt text only to fusion nodes and allows an empty fusion prompt', () => {
    const imageNode = makeImageNode()
    const fusionNode = makeFusionNode({ prompt: 'old' })

    expect(patchFlowSnapshotParam(imageNode, { key: 'prompt', value: 'new' })).toBe(imageNode)
    expect(patchFlowSnapshotParam(fusionNode, { key: 'prompt', value: '   ' })).toMatchObject({
      prompt: '',
    })
  })

  it('normalizes aspect ratio and input image patches', () => {
    const promptNode = makePromptNode()
    const promptImageInputLimit = getCanvasNodeImageInputLimit('prompt')
    const nextRatio = patchFlowSnapshotParam(promptNode, {
      key: 'aspectRatio',
      value: '16:9',
    })
    const nextImages = patchFlowSnapshotParam(promptNode, {
      key: 'inputImages',
      value: Array.from({ length: promptImageInputLimit + 1 }, (_, index) =>
        makeInputImage(`input-${index}`),
      ),
    })

    expect(nextRatio).toMatchObject({ aspectRatio: '16:9' })
    expect((nextImages as PromptNodeItem).inputImages).toHaveLength(promptImageInputLimit)
    expect((nextImages as PromptNodeItem).inputImages[0]?.mediaAssetId).toBe(301)
  })

  it('constrains fusion blend point when input image count changes', () => {
    const fusionImageInputLimit = getCanvasNodeImageInputLimit('fusion')
    const fusionNode = makeFusionNode({
      blendPoint: { x: 1, y: 1 },
      inputImages: Array.from({ length: fusionImageInputLimit }, (_, index) =>
        makeInputImage(`input-${index}`),
      ),
    })
    const next = patchFlowSnapshotParam(fusionNode, {
      key: 'inputImages',
      value: [makeInputImage('a')],
    }) as FusionNodeItem

    expect(next.inputImages).toHaveLength(1)
    expect(next.blendPoint).toEqual({ x: 0.5, y: 0.5 })
  })

  it('ignores blend point patches outside fusion nodes', () => {
    const promptNode = makePromptNode()

    expect(patchFlowSnapshotParam(promptNode, { key: 'blendPoint', value: { x: 0, y: 0 } })).toBe(
      promptNode,
    )
  })
})
