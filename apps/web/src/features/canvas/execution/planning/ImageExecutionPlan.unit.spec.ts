import { describe, expect, it } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { createImageExecutionPlanner, prepareFusionExecutionPlan } from './ImageExecutionPlan'
import {
  resolveFlowSourceImageRatio,
  resolveSourceImageFromNode,
} from '../../utils/flowNodeSourceResolution'

function makeImageNode(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 0.75,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

describe('ImageExecutionPlan', () => {
  it('uses the image node display output for connected image source data', () => {
    const snapshot = makeImageNode({
      outputImages: [
        { url: 'https://example.com/old.png', ratio: 1 },
        { url: 'https://example.com/latest.png', ratio: 1.5 },
      ],
    })

    expect(resolveSourceImageFromNode('image', snapshot)).toBe('https://example.com/latest.png')
    expect(resolveFlowSourceImageRatio('image', snapshot)).toBe(1.5)
  })

  it('falls back to source image data when an image node has no outputs', () => {
    const snapshot = makeImageNode({
      sourceImageUrl: ' https://example.com/source-only.png ',
      sourceImageRatio: 0.5,
      outputImages: [],
    })

    expect(resolveSourceImageFromNode('image', snapshot)).toBe(
      'https://example.com/source-only.png',
    )
    expect(resolveFlowSourceImageRatio('image', snapshot)).toBe(0.5)
  })

  it('does not treat source image fields as execution output fields for non-image nodes', () => {
    const snapshot = {
      type: 'fusion',
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 0.5,
      outputImages: [{ url: 'https://example.com/fusion.png', ratio: 1.25 }],
    }

    expect(resolveSourceImageFromNode('fusion', snapshot)).toBe('https://example.com/fusion.png')
    expect(resolveFlowSourceImageRatio('fusion', snapshot)).toBe(1.25)
  })

  it('plans Fusion inputs from graph edges, snapshot order, and connected prompt text', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-a',
          position: { x: 10, y: 20 },
          data: { kind: 'image', snapshotNode: makeImageNode({ id: 1 }) },
        },
        {
          id: 'image-b',
          position: { x: 30, y: 40 },
          data: {
            kind: 'image',
            snapshotNode: makeImageNode({
              id: 2,
              sourceImageUrl: 'https://example.com/b.png',
              sourceImageRatio: 2,
            }),
          },
        },
        {
          id: 'prompt-upstream',
          data: { kind: 'prompt', snapshotNode: { text: 'soft light' } },
        },
        {
          id: 'prompt-main',
          data: { kind: 'prompt', snapshotNode: { text: 'linen texture' } },
        },
        {
          id: 'fusion-1',
          position: { x: 100, y: 200 },
          data: {
            kind: 'fusion',
            snapshotNode: {
              id: 1,
              type: 'fusion',
              x: 0,
              y: 0,
              width: 360,
              prompt: '',
              outputCount: 2,
              aspectRatio: 'auto',
              blendPoint: { x: 0.5, y: 0.5 },
              inputImages: [
                {
                  id: 'snapshot-b',
                  sourceType: 'node',
                  sourceId: 'image-b',
                  url: 'stale-b.png',
                  ratio: 1,
                },
                {
                  id: 'external',
                  sourceType: 'external',
                  sourceId: 'external-1',
                  url: ' https://example.com/external.png ',
                  ratio: 1.25,
                },
              ],
              outputStatus: 'idle',
              outputImages: [],
            },
          },
        },
      ],
      edges: [
        { source: 'image-a', target: 'fusion-1', targetHandle: 'in:image' },
        { source: 'image-b', target: 'fusion-1', targetHandle: 'in:image' },
        { source: 'prompt-upstream', target: 'prompt-main', targetHandle: 'in:prompt' },
        { source: 'prompt-main', target: 'fusion-1', targetHandle: 'in:prompt' },
      ],
    }))

    const plan = planner.planFusionPromptGeneration('fusion-1', '', 4, '16:9')

    expect(plan.kind).toBe('fusion')
    expect(plan.outputCount).toBe(4)
    expect(plan.aspectRatio).toBe('16:9')
    expect(plan.imageUrls).toEqual([
      'https://example.com/b.png',
      'https://example.com/external.png',
      'https://example.com/source.png',
    ])
    expect(plan.taskPrompt).toBe('[style-fusion] soft light\nlinen texture')
    expect(plan.source).toEqual({ url: 'https://example.com/b.png', ratio: 2 })
    expect(plan.resultOptions.anchor).toEqual({
      x: 100,
      y: 200,
      width: 360,
      outputAnchorNodeId: 'fusion-1',
    })

    expect(() => planner.planFusionPromptGeneration('fusion-1', '', 9, '16:9')).toThrow(
      '生成数量无效',
    )
  })

  it('keeps execution input separate from result anchor when planning image actions', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-1',
          position: { x: 12, y: 24 },
          data: {
            kind: 'image',
            snapshotNode: makeImageNode({
              width: 320,
              sourceImageUrl: ' https://example.com/source.png ',
              sourceImageRatio: 0.75,
            }),
          },
        },
      ],
      edges: [],
    }))

    const plan = planner.planImageDirectAction('image-1', 'remove-bg')

    expect(plan.source).toEqual({ url: 'https://example.com/source.png', ratio: 0.75 })
    expect(plan.source).not.toHaveProperty('x')
    expect(plan.source).not.toHaveProperty('width')
    expect(plan.resultOptions.anchor).toEqual({
      x: 12,
      y: 24,
      width: 320,
      outputAnchorNodeId: 'image-1',
    })
  })

  it('rejects direct image actions when the image node has no URL', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-empty',
          data: {
            kind: 'image',
            snapshotNode: makeImageNode({ sourceImageUrl: '', outputImages: [] }),
          },
        },
      ],
      edges: [],
    }))

    expect(() => planner.planImageDirectAction('image-empty', 'remove-bg')).toThrow(
      '图片节点不存在或缺少 URL',
    )
  })

  it('rejects inpaint plans with empty prompt or mask before preparing media', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-1',
          data: { kind: 'image', snapshotNode: makeImageNode() },
        },
      ],
      edges: [],
    }))

    expect(() =>
      planner.planImageInpaintAction('image-1', {
        prompt: '   ',
        maskDataUrl: 'data:image/png;base64,mask',
        outputCount: 1,
      }),
    ).toThrow('请输入 Prompt')
    expect(() =>
      planner.planImageInpaintAction('image-1', {
        prompt: 'redraw',
        maskDataUrl: '   ',
        outputCount: 1,
      }),
    ).toThrow('请先标记局部重绘区域')
  })

  it('rejects canvas extend plans with no extension', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-1',
          data: { kind: 'image', snapshotNode: makeImageNode() },
        },
      ],
      edges: [],
    }))

    expect(() =>
      planner.planCanvasExtendAction('image-1', {
        frameRect: { left: 0, top: 0, width: 100, height: 100 },
        originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      }),
    ).toThrow('请先拖动扩图框')
  })

  it('normalizes prompt aspect ratios for image and text prompt plans', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-1',
          data: { kind: 'image', snapshotNode: makeImageNode() },
        },
        {
          id: 'prompt-1',
          position: { x: 40, y: 80 },
          data: { kind: 'prompt', snapshotNode: { width: 360 } },
        },
      ],
      edges: [],
    }))

    const imagePlan = planner.planImagePromptGeneration(
      'image-1',
      'generate',
      1,
      'bad-ratio' as never,
    )
    const textPlan = planner.planPromptGeneration('prompt-1', [], 'generate', 1, 'bad' as never)

    expect(imagePlan.aspectRatio).toBe('auto')
    expect(textPlan.aspectRatio).toBe('auto')
    expect(textPlan.resultOptions.anchor).toEqual({
      x: 40,
      y: 80,
      width: 360,
      outputAnchorNodeId: 'prompt-1',
    })
  })

  it('uses output anchor override only for result layout on image prompt plans', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-1',
          position: { x: 10, y: 20 },
          data: { kind: 'image', snapshotNode: makeImageNode({ width: 320 }) },
        },
        {
          id: 'prompt-1',
          position: { x: 200, y: 300 },
          data: { kind: 'prompt', snapshotNode: { width: 420 } },
        },
      ],
      edges: [],
    }))

    const plan = planner.planImagePromptGeneration('image-1', 'generate', 1, '1:1', {
      outputAnchorNodeId: 'prompt-1',
    })

    expect(plan.source).toEqual({ url: 'https://example.com/source.png', ratio: 0.75 })
    expect(plan.resultOptions.anchor).toEqual({
      x: 200,
      y: 300,
      width: 420,
      outputAnchorNodeId: 'prompt-1',
    })
  })

  it('prepares Fusion sizing and keeps the prepared request source layout-free', () => {
    const planner = createImageExecutionPlanner(() => ({
      nodes: [
        {
          id: 'image-a',
          data: { kind: 'image', snapshotNode: makeImageNode({ id: 1 }) },
        },
        {
          id: 'image-b',
          data: {
            kind: 'image',
            snapshotNode: makeImageNode({
              id: 2,
              sourceImageUrl: 'https://example.com/b.png',
            }),
          },
        },
        {
          id: 'fusion-1',
          position: { x: 10, y: 20 },
          data: {
            kind: 'fusion',
            snapshotNode: {
              id: 1,
              type: 'fusion',
              x: 0,
              y: 0,
              width: 320,
              prompt: '',
              outputCount: 1,
              aspectRatio: 'auto',
              blendPoint: { x: 0.5, y: 0.5 },
              inputImages: [],
              outputStatus: 'idle',
              outputImages: [],
            },
          },
        },
      ],
      edges: [
        { source: 'image-a', target: 'fusion-1', targetHandle: 'in:image' },
        { source: 'image-b', target: 'fusion-1', targetHandle: 'in:image' },
      ],
    }))

    const plan = planner.planFusionPromptGeneration('fusion-1', 'blend', 1, '16:9')
    const prepared = prepareFusionExecutionPlan(plan, [
      { width: 1000, height: 1000 },
      { width: 1600, height: 900 },
    ])

    expect(prepared.source).toEqual({
      url: 'https://example.com/source.png',
      ratio: 9 / 16,
    })
    expect(prepared.source).not.toHaveProperty('x')
    expect(prepared.targetSize).toEqual({ width: 1473, height: 829 })
    expect(prepared.resultOptions).toMatchObject({
      anchor: {
        x: 10,
        y: 20,
        width: 320,
        outputAnchorNodeId: 'fusion-1',
      },
      targetRatio: 9 / 16,
      targetWidth: 462,
    })
  })
})
