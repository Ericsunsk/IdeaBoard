import { describe, expect, it } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { getCanvasNodeImageInputLimit } from '../../../canvas-board/domain'
import {
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  resolveHdClarityStrength,
} from '../contracts/imageActionExecution'
import { EXECUTION_FEATURE } from '../contracts/executionContract'
import {
  buildCanvasExtendTaskRequest,
  buildDirectActionTaskRequest,
  buildFusionTaskRequest,
  buildHdUpscaleTaskRequest,
  buildImageActionReadinessRequest,
  buildImageInpaintTaskRequest,
} from './flowImageTaskRequests'

const source = {
  url: 'https://assets.example.com/source.png',
  width: 280,
  ratio: 1,
  x: 0,
  y: 0,
}

describe('flowImageTaskRequests', () => {
  it('normalizes HD clarity strength without changing size semantics', () => {
    expect(resolveHdClarityStrength(undefined)).toBe(50)
    expect(resolveHdClarityStrength(75.4)).toBe(75)
  })

  it('builds task readiness requests from the execution contract', () => {
    expect(buildImageActionReadinessRequest(IMAGE_NODE_ACTION.styleVariation)).toMatchObject({
      prompt: 'submit readiness',
      feature: EXECUTION_FEATURE.styleVariation,
      params: {
        variation: {
          strength: 50,
        },
      },
    })
    expect(buildImageActionReadinessRequest(IMAGE_HD_UPSCALE_ACTION)).toMatchObject({
      prompt: 'submit readiness',
      feature: EXECUTION_FEATURE.hdUpscale,
      params: {
        hd: {
          clarity_strength: 50,
        },
      },
    })
  })

  it('builds style variation params under the nested variation contract', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.styleVariation,
      imageSize: { width: 1024, height: 1024 },
      strength: 61.6,
    })

    expect(request.params).toMatchObject({
      custom_width: 1024,
      custom_height: 1024,
      original_image_width: 1024,
      original_image_height: 1024,
      variation: {
        strength: 62,
      },
    })
    expect(request.params).not.toHaveProperty('variation_strength')
  })

  it('keeps remove-bg submit params limited to image sizing', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.removeBg,
      imageSize: { width: 1024, height: 1024 },
    })

    expect(request.params).toMatchObject({
      custom_width: 1024,
      custom_height: 1024,
      original_image_width: 1024,
      original_image_height: 1024,
    })
    expect(request.params).not.toHaveProperty('output_format')
  })

  it('keeps HD output size at the source size and maps strength to quality', () => {
    const request = buildHdUpscaleTaskRequest({
      source,
      imageSize: { width: 1200, height: 800 },
      strength: 75,
    })

    expect(request.params).toMatchObject({
      custom_width: 1200,
      custom_height: 800,
      original_image_width: 1200,
      original_image_height: 800,
      hd: {
        clarity_strength: 75,
      },
    })
    expect(request.params?.hd).not.toHaveProperty('output_quality')
    expect(request.params?.hd).not.toHaveProperty('size_policy')
    expect(request.params).not.toHaveProperty('upscale_factor')
    expect(request.params).not.toHaveProperty('upscale_mode')
  })

  it('limits inpaint reference images by the canvas image input contract', () => {
    const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
    const referenceImageUrls = Array.from(
      { length: referenceImageLimit + 2 },
      (_, index) => ` https://assets.example.com/ref-${index}.png `,
    )

    const request = buildImageInpaintTaskRequest({
      prompt: 'redraw the selected area',
      croppedImageUrl: 'https://assets.example.com/crop.png',
      croppedMaskUrl: 'https://assets.example.com/mask.png',
      referenceImageUrls,
      cropSize: { width: 512, height: 384 },
    })

    expect(request.params).toMatchObject({
      reference_image_urls: referenceImageUrls
        .map((url) => url.trim())
        .slice(0, referenceImageLimit),
      use_mask: true,
    })
  })

  it('builds image extend params under the nested extend contract', () => {
    const request = buildCanvasExtendTaskRequest({
      source,
      imageSize: { width: 1000, height: 750 },
      extendPercent: {
        top: 10,
        right: 20,
        bottom: 30,
        left: 40,
      },
      prompt: 'extend horizon',
    })

    expect(request.feature).toBe(EXECUTION_FEATURE.imageExtend)
    expect(request.params).toMatchObject({
      custom_width: 1600,
      custom_height: 1050,
      original_image_width: 1000,
      original_image_height: 750,
      extend: {
        top_percent: 10,
        right_percent: 20,
        bottom_percent: 30,
        left_percent: 40,
      },
    })
    expect(request.params).not.toHaveProperty('extend_top')
    expect(request.params).not.toHaveProperty('extend_right')
    expect(request.params).not.toHaveProperty('extend_bottom')
    expect(request.params).not.toHaveProperty('extend_left')
  })

  it('builds canvas extend readiness from the same action contract', () => {
    expect(buildImageActionReadinessRequest(IMAGE_CANVAS_EXTEND_ACTION)).toMatchObject({
      feature: EXECUTION_FEATURE.imageExtend,
      params: {
        extend: {
          right_percent: 1,
        },
      },
    })
  })

  it('builds Fusion weighted reference params without submitting', () => {
    const request = buildFusionTaskRequest({
      prompt: '[style-fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.35, 0.65],
      sizeSource: null,
      targetSize: null,
      aspectRatio: 'auto',
    })

    expect(request).toMatchObject({
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.35, 0.65],
        },
      },
    })
  })

  it('builds Fusion weighted reference params with size and aspect ratio', () => {
    const request = buildFusionTaskRequest({
      prompt: '[style-fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.4, 0.6],
      sizeSource: { width: 1024, height: 768 },
      targetSize: { width: 1280, height: 720 },
      aspectRatio: '16:9',
    })

    expect(request).toMatchObject({
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        custom_width: 1280,
        custom_height: 720,
        original_image_width: 1024,
        original_image_height: 768,
        ratio: '16:9',
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.4, 0.6],
        },
      },
    })

    expect((request as SubmitTaskData).params).toBeDefined()
  })
})
