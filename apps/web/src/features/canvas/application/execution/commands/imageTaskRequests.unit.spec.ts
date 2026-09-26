import { describe, expect, it } from 'vitest'
import type { SubmitTaskData } from '../../../domain/execution-contracts/canvasTaskRequest'
import { IMAGE_NODE_ACTION } from '../../../../../shared/domain/imageActions'
import {
  IMAGE_OUTPAINT_ACTION,
  IMAGE_UPSCALE_ACTION,
  resolveUpscaleScale,
} from '../../../domain/execution-contracts/imageActionExecution.ts'
import { EXECUTION_FEATURE } from '../../../domain/execution-contracts/executionContract.ts'
import type {
  OutpaintExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  PreparedFusionExecutionPlan,
  GenerationExecutionPlan,
} from '../planning/imageExecutionPlan'
import {
  buildOutpaintTaskRequest,
  buildDirectActionTaskRequest,
  buildFusionTaskRequest,
  buildUpscaleTaskRequest,
  buildImageExecutionTaskRequest,
  buildImageActionReadinessRequest,
  buildImageInpaintTaskRequest,
  buildInpaintReadinessRequest,
  buildOutpaintReadinessRequest,
  buildPreparedImageInpaintTaskRequest,
  buildGenerationTaskRequest,
} from './imageTaskRequests'

const source = {
  url: 'https://assets.example.com/source.png',
  ratio: 1,
}

describe('imageTaskRequests', () => {
  it('prefers AssetID over runtime URL for internal Canvas sources', () => {
    const request = buildDirectActionTaskRequest({
      source: { ...source, mediaAssetId: 42 },
      action: IMAGE_NODE_ACTION.variations,
      imageSize: null,
      prompt: 'vary this image',
      outputCount: 1,
    })

    expect(request.image_asset_ids).toEqual([42])
    expect(request).not.toHaveProperty('image_urls')
  })

  it('normalizes upscale slider strength to a 1.0-4.0 scale', () => {
    expect(resolveUpscaleScale(undefined)).toBe(2)
    expect(resolveUpscaleScale(0)).toBe(1)
    expect(resolveUpscaleScale(2.34)).toBe(2.3)
    expect(resolveUpscaleScale(5)).toBe(4)
  })

  it('builds task readiness requests from the execution contract', () => {
    expect(buildImageActionReadinessRequest(IMAGE_NODE_ACTION.variations)).toMatchObject({
      prompt: '',
      feature: EXECUTION_FEATURE.variations,
      params: {
        variation: {
          strength: 50,
        },
      },
    })
    expect(buildImageActionReadinessRequest(IMAGE_UPSCALE_ACTION)).toMatchObject({
      prompt: '',
      feature: EXECUTION_FEATURE.upscale,
      params: {
        upscale: {
          scale: 2,
        },
      },
    })
    expect(
      buildImageActionReadinessRequest(IMAGE_NODE_ACTION.variations, {
        outputCount: 4,
        outputResolution: '4k',
      }),
    ).toMatchObject({
      params: {
        output_count: 4,
        output_resolution: '4k',
      },
    })
    expect(
      buildImageActionReadinessRequest(IMAGE_NODE_ACTION.cutout, {
        outputCount: 4,
        outputResolution: '4k',
      }),
    ).toMatchObject({
      params: { output_count: 1, output_resolution: 'default' },
    })
  })

  it('builds valid quote-only fill readiness placeholders', () => {
    expect(buildInpaintReadinessRequest()).toMatchObject({
      prompt: '[inpaint] submit readiness',
      feature: EXECUTION_FEATURE.inpaint,
      params: {
        output_count: 1,
        output_resolution: 'default',
        original_image_width: 1024,
        original_image_height: 1024,
        custom_width: 1024,
        custom_height: 1024,
      },
    })
    expect(buildOutpaintReadinessRequest()).toMatchObject({
      prompt: '',
      feature: EXECUTION_FEATURE.outpaint,
      params: {
        output_count: 1,
        output_resolution: 'default',
        original_image_width: 1024,
        original_image_height: 1024,
        outpaint: { target_width: 1032, target_height: 1024, image_offset_x: 0, image_offset_y: 0 },
      },
    })
  })

  it('builds generation requests with output count independent from ordered refs', () => {
    const request = buildGenerationTaskRequest({
      prompt: '  repeatable textile pattern  ',
      aspectRatio: '1:1',
      imageUrls: ['https://assets.example.com/ref-1.png', 'https://assets.example.com/ref-2.png'],
      outputCount: 3,
    })

    expect(request).toEqual({
      prompt: 'repeatable textile pattern',
      priority: 1,
      feature: EXECUTION_FEATURE.generation,
      image_urls: ['https://assets.example.com/ref-1.png', 'https://assets.example.com/ref-2.png'],
      params: { output_count: 3, output_resolution: '1k', ratio: '1:1' },
    })
  })

  it('builds style variation params under the nested variation contract', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.variations,
      imageSize: { width: 1024, height: 1024 },
      prompt: 'vary style',
      outputCount: 1,
      strength: 61.6,
      outputResolution: '2k',
    })

    expect(request.params).toMatchObject({
      custom_width: 1024,
      custom_height: 1024,
      original_image_width: 1024,
      original_image_height: 1024,
      variation: {
        strength: 62,
      },
      output_resolution: '2k',
    })
    expect(request.params).not.toHaveProperty('variation_strength')
  })

  it('rejects direct action requests without an image URL', () => {
    expect(() =>
      buildDirectActionTaskRequest({
        source: { url: '   ', ratio: 1 },
        action: IMAGE_NODE_ACTION.cutout,
        imageSize: null,
        prompt: '',
        outputCount: 1,
      }),
    ).toThrow('The image is missing a URL.')
  })

  it('keeps cutout submit params limited to image sizing', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.cutout,
      imageSize: { width: 1024, height: 1024 },
      prompt: '',
      outputCount: 1,
    })

    expect(request.params).toMatchObject({
      output_count: 1,
      output_resolution: 'default',
      custom_width: 1024,
      custom_height: 1024,
      original_image_width: 1024,
      original_image_height: 1024,
    })
    expect(request.params).not.toHaveProperty('output_format')
  })

  it('keeps upscale output size at the source size and sends only scale', () => {
    const request = buildUpscaleTaskRequest({
      source,
      imageSize: { width: 1200, height: 800 },
      prompt: 'ignored prompt',
      outputCount: 1,
      strength: 3.2,
    })

    expect(request.params).toMatchObject({
      custom_width: 1200,
      custom_height: 800,
      original_image_width: 1200,
      original_image_height: 800,
      upscale: {
        scale: 3.2,
      },
    })
    expect(request.params?.upscale).not.toHaveProperty('output_quality')
    expect(request.params?.upscale).not.toHaveProperty('size_policy')
    expect(request.params).not.toHaveProperty('upscale_factor')
    expect(request.params).not.toHaveProperty('upscale_mode')
    expect(request.params).not.toHaveProperty('output_count')
    expect(request.params).not.toHaveProperty('output_resolution')
    expect(request.prompt).toBe('')
  })

  it('prefers the media asset ID over an empty runtime URL for upscale', () => {
    const request = buildUpscaleTaskRequest({
      source: { ...source, mediaAssetId: 42 },
      imageSize: { width: 1200, height: 800 },
      prompt: '',
      outputCount: 1,
    })

    expect(request.image_asset_ids).toEqual([42])
    expect(request).not.toHaveProperty('image_urls')
  })

  it('keeps each image workflow in one 1-4 output task with the optional prompt', () => {
    const prompt = '  preserve the subject  '
    const requests = [
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.variations,
        imageSize: null,
        prompt,
        outputCount: 4,
        strength: 35,
      }),
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.render,
        imageSize: null,
        prompt,
        outputCount: 4,
      }),
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.cutout,
        imageSize: null,
        prompt,
        outputCount: 4,
      }),
      buildUpscaleTaskRequest({
        source,
        imageSize: null,
        prompt,
        outputCount: 4,
        strength: 65,
      }),
    ]

    expect(requests.map((request) => request.feature)).toEqual([
      EXECUTION_FEATURE.variations,
      EXECUTION_FEATURE.render,
      EXECUTION_FEATURE.cutout,
      EXECUTION_FEATURE.upscale,
    ])
    for (const request of requests) {
      expect(request.image_urls).toEqual([source.url])
      if (request.feature === EXECUTION_FEATURE.upscale) {
        expect(request.prompt).toBe('')
        expect(request.params).not.toHaveProperty('output_count')
        expect(request.params).not.toHaveProperty('output_resolution')
        continue
      }
      expect(request.prompt).toBe('preserve the subject')
      expect(request.params?.output_count).toBe(
        request.feature === EXECUTION_FEATURE.cutout ? 1 : 4,
      )
      if (request.feature === EXECUTION_FEATURE.cutout) {
        expect(request.params?.output_resolution).toBe('default')
      }
    }
  })

  it('builds single-output inpaint params with source dimensions and no reference contract', () => {
    const request = buildImageInpaintTaskRequest({
      prompt: 'redraw the selected area',
      originalImageUrl: source.url,
      maskImageUrl: 'https://assets.example.com/mask.png',
      imageSize: { width: 1200, height: 800 },
    })

    expect(request.params).toMatchObject({
      output_count: 1,
      custom_width: 1200,
      custom_height: 800,
      original_image_width: 1200,
      original_image_height: 800,
      use_mask: true,
    })
    expect(request.params).not.toHaveProperty('reference_image_urls')
  })

  it('emits mask_asset_id and drops mask_image_url once the mask is a workspace asset', () => {
    const request = buildImageInpaintTaskRequest({
      prompt: 'redraw the selected area',
      originalImageUrl: source.url,
      maskImageUrl: 'https://assets.example.com/mask.png',
      maskAssetId: ' asset_mask1 ',
      imageSize: { width: 1200, height: 800 },
      originalImageMediaAssetId: 'asset_aaa1',
    })

    expect(request.feature).toBe(EXECUTION_FEATURE.inpaint)
    expect(request.media_asset_ids).toEqual(['asset_aaa1'])
    expect(request.params).toMatchObject({
      output_count: 1,
      mask_asset_id: 'asset_mask1',
      use_mask: true,
    })
    expect(request.params).not.toHaveProperty('mask_image_url')
  })

  it('builds outpaint params under the nested outpaint contract', () => {
    const request = buildOutpaintTaskRequest({
      source,
      imageSize: { width: 1000, height: 750 },
      frameRect: { left: -40, top: -10, width: 160, height: 140 },
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      prompt: 'extend horizon',
    })

    expect(request.feature).toBe(EXECUTION_FEATURE.outpaint)
    expect(request.params).toMatchObject({
      output_count: 1,
      output_resolution: 'default',
      original_image_width: 1000,
      original_image_height: 750,
      outpaint: {
        target_width: 1600,
        target_height: 1050,
        image_offset_x: 400,
        image_offset_y: 75,
      },
    })
    expect(request.params).not.toHaveProperty('custom_width')
    expect(request.params).not.toHaveProperty('custom_height')
  })

  it('builds outpaint readiness from the same action contract', () => {
    expect(buildImageActionReadinessRequest(IMAGE_OUTPAINT_ACTION)).toMatchObject({
      feature: EXECUTION_FEATURE.outpaint,
      params: {
        outpaint: {
          target_width: 1032,
        },
      },
    })
  })

  it('builds Fusion weighted reference params without submitting', () => {
    const request = buildFusionTaskRequest({
      prompt: '[fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.35, 0.65],
      outputCount: 3,
      outputResolution: '4k',
      sizeSource: null,
      targetSize: null,
      aspectRatio: 'auto',
    })

    expect(request).toMatchObject({
      prompt: '[fusion] blend references',
      feature: EXECUTION_FEATURE.fusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        output_count: 3,
        output_resolution: '4k',
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.35, 0.65],
        },
      },
    })
  })

  it('builds Fusion weighted reference params with size and aspect ratio', () => {
    const request = buildFusionTaskRequest({
      prompt: '[fusion] blend references',
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      inputWeights: [0.4, 0.6],
      sizeSource: { width: 1024, height: 768 },
      targetSize: { width: 1280, height: 720 },
      aspectRatio: '16:9',
    })

    expect(request).toMatchObject({
      prompt: '[fusion] blend references',
      feature: EXECUTION_FEATURE.fusion,
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

  it('adapts a prepared Fusion execution plan to the backend request contract', () => {
    const plan = {
      kind: 'fusion',
      sourceNodeId: 'fusion-1',
      source,
      taskPrompt: '[fusion] blend references',
      outputCount: 2,
      aspectRatio: '4:3',
      inputImages: [],
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      blendPoint: { x: 0.5, y: 0.5 },
      stageWidth: 280,
      inputWeights: [0.25, 0.75],
      sizeSource: { width: 900, height: 600 },
      targetSize: { width: 1200, height: 900 },
      targetRatio: 0.75,
      resultOptions: {
        anchor: {
          x: 0,
          y: 0,
          width: 280,
          outputAnchorNodeId: 'fusion-1',
        },
        targetRatio: 0.75,
        targetWidth: 300,
      },
    } satisfies PreparedFusionExecutionPlan

    expect(buildImageExecutionTaskRequest(plan)).toMatchObject({
      prompt: '[fusion] blend references',
      feature: EXECUTION_FEATURE.fusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        custom_width: 1200,
        custom_height: 900,
        original_image_width: 900,
        original_image_height: 600,
        ratio: '4:3',
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.25, 0.75],
        },
      },
    })
  })

  it('builds image execution task requests for direct action, prompt, text, outpaint, and fusion branches', () => {
    const actionPlan: ImageActionExecutionPlan = {
      kind: 'image-action',
      sourceNodeId: 'image-1',
      source,
      action: IMAGE_NODE_ACTION.cutout,
      prompt: '',
      outputCount: 1,
      resultOptions: {},
    }
    const textPlan: GenerationExecutionPlan = {
      kind: 'generation',
      sourceNodeId: 'prompt-1',
      source: { url: '', ratio: 1 },
      referenceImages: [],
      prompt: 'make a pattern',
      outputCount: 1,
      aspectRatio: 'auto',
      resultOptions: {},
    }
    const outpaintPlan: OutpaintExecutionPlan = {
      kind: 'outpaint',
      sourceNodeId: 'image-1',
      source,
      prompt: 'extend horizon',
      outputCount: 1,
      frameRect: { left: 0, top: 0, width: 110, height: 100 },
      originalImageRect: { left: 0, top: 0, width: 100, height: 100 },
      targetRatio: 1,
      targetWidth: 300,
      resultOptions: {},
    }
    const fusionPlan: PreparedFusionExecutionPlan = {
      kind: 'fusion',
      sourceNodeId: 'fusion-1',
      source,
      taskPrompt: '[fusion] blend',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
      imageUrls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      blendPoint: { x: 0.5, y: 0.5 },
      stageWidth: 320,
      inputWeights: [0.5, 0.5],
      sizeSource: null,
      targetSize: null,
      targetRatio: 1,
      resultOptions: {},
    }

    expect(buildImageExecutionTaskRequest(actionPlan).feature).toBe(EXECUTION_FEATURE.cutout)
    expect(buildImageExecutionTaskRequest(textPlan)).toMatchObject({
      feature: EXECUTION_FEATURE.generation,
    })
    expect(buildImageExecutionTaskRequest(textPlan)).not.toHaveProperty('image_urls')
    expect(buildImageExecutionTaskRequest(textPlan)).toMatchObject({ params: { output_count: 1 } })
    expect(
      buildImageExecutionTaskRequest(outpaintPlan, { width: 1000, height: 1000 }),
    ).toMatchObject({
      feature: EXECUTION_FEATURE.outpaint,
      image_urls: [source.url],
      params: {
        outpaint: {
          target_width: 1100,
          target_height: 1000,
        },
      },
    })
    expect(buildImageExecutionTaskRequest(fusionPlan)).toMatchObject({
      feature: EXECUTION_FEATURE.fusion,
      image_urls: ['https://assets.example.com/a.png', 'https://assets.example.com/b.png'],
      params: {
        fusion: {
          input_weights: [0.5, 0.5],
        },
      },
    })
  })

  it('uses the prepared inpaint request builder for image inpaint plans', () => {
    const plan: ImageInpaintExecutionPlan = {
      kind: 'inpaint',
      sourceNodeId: 'image-1',
      source: { ...source, mediaAssetId: 42 },
      prompt: 'replace object',
      rawMaskDataUrl: 'data:image/png;base64,mask',
      outputCount: 1,
      resultOptions: {},
    }

    expect(
      buildPreparedImageInpaintTaskRequest({
        plan,
        maskImageUrl: 'https://assets.example.com/mask.png',
        imageSize: { width: 900, height: 600 },
      }),
    ).toMatchObject({
      feature: EXECUTION_FEATURE.inpaint,
      params: {
        output_count: 1,
        original_image_asset_id: 42,
        mask_image_url: 'https://assets.example.com/mask.png',
        custom_width: 900,
        custom_height: 600,
        original_image_width: 900,
        original_image_height: 600,
        use_mask: true,
      },
    })
    expect(
      buildPreparedImageInpaintTaskRequest({
        plan,
        maskImageUrl: 'https://assets.example.com/mask.png',
        imageSize: { width: 900, height: 600 },
      }).params,
    ).not.toHaveProperty('reference_image_urls')
  })
})
