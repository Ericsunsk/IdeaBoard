import { describe, expect, it } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { getCanvasNodeImageInputLimit } from '../../../canvas-board/domain'
import {
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  resolveHdScale,
} from '../contracts/imageActionExecution'
import { EXECUTION_FEATURE } from '../contracts/executionContract'
import type {
  CanvasExtendExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  PreparedFusionExecutionPlan,
  PromptGenerationExecutionPlan,
} from '../planning/ImageExecutionPlan'
import {
  buildCanvasExtendTaskRequest,
  buildDirectActionTaskRequest,
  buildFusionTaskRequest,
  buildHdUpscaleTaskRequest,
  buildImageExecutionTaskRequest,
  buildImageActionReadinessRequest,
  buildImageInpaintTaskRequest,
  buildPreparedImageInpaintTaskRequest,
  buildPromptGenerationTaskRequest,
} from './flowImageTaskRequests'

const source = {
  url: 'https://assets.example.com/source.png',
  ratio: 1,
}

describe('flowImageTaskRequests', () => {
  it('normalizes HD slider strength to a 1.0-4.0 scale', () => {
    expect(resolveHdScale(undefined)).toBe(2)
    expect(resolveHdScale(0)).toBe(1)
    expect(resolveHdScale(2.34)).toBe(2.3)
    expect(resolveHdScale(5)).toBe(4)
  })

  it('builds task readiness requests from the execution contract', () => {
    expect(buildImageActionReadinessRequest(IMAGE_NODE_ACTION.styleVariation)).toMatchObject({
      prompt: '',
      feature: EXECUTION_FEATURE.styleVariation,
      params: {
        variation: {
          strength: 50,
        },
      },
    })
    expect(buildImageActionReadinessRequest(IMAGE_HD_UPSCALE_ACTION)).toMatchObject({
      prompt: '',
      feature: EXECUTION_FEATURE.hdUpscale,
      params: {
        hd: {
          scale: 2,
        },
      },
    })
    expect(
      buildImageActionReadinessRequest(IMAGE_NODE_ACTION.styleVariation, {
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
      buildImageActionReadinessRequest(IMAGE_NODE_ACTION.removeBg, {
        outputCount: 4,
        outputResolution: '4k',
      }),
    ).toMatchObject({
      params: { output_count: 1, output_resolution: 'default' },
    })
  })

  it('builds prompt-generation requests with output count independent from ordered refs', () => {
    const request = buildPromptGenerationTaskRequest({
      prompt: '  repeatable textile pattern  ',
      aspectRatio: '1:1',
      imageUrls: ['https://assets.example.com/ref-1.png', 'https://assets.example.com/ref-2.png'],
      outputCount: 3,
    })

    expect(request).toEqual({
      prompt: 'repeatable textile pattern',
      priority: 1,
      feature: EXECUTION_FEATURE.promptGeneration,
      image_urls: ['https://assets.example.com/ref-1.png', 'https://assets.example.com/ref-2.png'],
      params: { output_count: 3, output_resolution: '1k', ratio: '1:1' },
    })
  })

  it('builds style variation params under the nested variation contract', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.styleVariation,
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
        action: IMAGE_NODE_ACTION.removeBg,
        imageSize: null,
        prompt: '',
        outputCount: 1,
      }),
    ).toThrow('图片缺少 URL')
  })

  it('keeps remove-bg submit params limited to image sizing', () => {
    const request = buildDirectActionTaskRequest({
      source,
      action: IMAGE_NODE_ACTION.removeBg,
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

  it('keeps HD output size at the source size and sends only scale', () => {
    const request = buildHdUpscaleTaskRequest({
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
      hd: {
        scale: 3.2,
      },
    })
    expect(request.params?.hd).not.toHaveProperty('output_quality')
    expect(request.params?.hd).not.toHaveProperty('size_policy')
    expect(request.params).not.toHaveProperty('upscale_factor')
    expect(request.params).not.toHaveProperty('upscale_mode')
    expect(request.params).not.toHaveProperty('output_count')
    expect(request.params).not.toHaveProperty('output_resolution')
    expect(request.prompt).toBe('')
  })

  it('keeps each image workflow in one 1-4 output task with the optional prompt', () => {
    const prompt = '  preserve the subject  '
    const requests = [
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.styleVariation,
        imageSize: null,
        prompt,
        outputCount: 4,
        strength: 35,
      }),
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.sketchToImage,
        imageSize: null,
        prompt,
        outputCount: 4,
      }),
      buildDirectActionTaskRequest({
        source,
        action: IMAGE_NODE_ACTION.removeBg,
        imageSize: null,
        prompt,
        outputCount: 4,
      }),
      buildHdUpscaleTaskRequest({
        source,
        imageSize: null,
        prompt,
        outputCount: 4,
        strength: 65,
      }),
    ]

    expect(requests.map((request) => request.feature)).toEqual([
      EXECUTION_FEATURE.styleVariation,
      EXECUTION_FEATURE.sketchToImage,
      EXECUTION_FEATURE.removeBg,
      EXECUTION_FEATURE.hdUpscale,
    ])
    for (const request of requests) {
      expect(request.image_urls).toEqual([source.url])
      if (request.feature === EXECUTION_FEATURE.hdUpscale) {
        expect(request.prompt).toBe('')
        expect(request.params).not.toHaveProperty('output_count')
        expect(request.params).not.toHaveProperty('output_resolution')
        continue
      }
      expect(request.prompt).toBe('preserve the subject')
      expect(request.params?.output_count).toBe(
        request.feature === EXECUTION_FEATURE.removeBg ? 1 : 4,
      )
      if (request.feature === EXECUTION_FEATURE.removeBg) {
        expect(request.params?.output_resolution).toBe('default')
      }
    }
  })

  it('limits inpaint reference images by the canvas image input contract', () => {
    const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
    const referenceImageUrls = Array.from(
      { length: referenceImageLimit + 2 },
      (_, index) => ` https://assets.example.com/ref-${index}.png `,
    )

    const request = buildImageInpaintTaskRequest({
      prompt: 'redraw the selected area',
      originalImageUrl: source.url,
      maskImageUrl: 'https://assets.example.com/mask.png',
      referenceImageUrls,
      outputCount: 2,
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
      outputCount: 3,
      outputResolution: '4k',
    })

    expect(request.feature).toBe(EXECUTION_FEATURE.imageExtend)
    expect(request.params).toMatchObject({
      output_count: 3,
      output_resolution: '4k',
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
      outputCount: 3,
      outputResolution: '4k',
      sizeSource: null,
      targetSize: null,
      aspectRatio: 'auto',
    })

    expect(request).toMatchObject({
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
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

  it('adapts a prepared Fusion execution plan to the backend request contract', () => {
    const plan = {
      kind: 'fusion',
      sourceNodeId: 'fusion-1',
      source,
      taskPrompt: '[style-fusion] blend references',
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
      prompt: '[style-fusion] blend references',
      feature: EXECUTION_FEATURE.styleFusion,
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

  it('builds image execution task requests for direct action, prompt, text, extend, and fusion branches', () => {
    const actionPlan: ImageActionExecutionPlan = {
      kind: 'image-action',
      sourceNodeId: 'image-1',
      source,
      action: IMAGE_NODE_ACTION.removeBg,
      prompt: '',
      outputCount: 1,
      resultOptions: {},
    }
    const textPlan: PromptGenerationExecutionPlan = {
      kind: 'prompt-generation',
      sourceNodeId: 'prompt-1',
      source: { url: '', ratio: 1 },
      referenceImages: [],
      prompt: 'make a pattern',
      outputCount: 1,
      aspectRatio: 'auto',
      resultOptions: {},
    }
    const extendPlan: CanvasExtendExecutionPlan = {
      kind: 'canvas-extend',
      sourceNodeId: 'image-1',
      source,
      prompt: 'extend horizon',
      outputCount: 1,
      extendPercent: { top: 0, right: 10, bottom: 0, left: 0 },
      targetRatio: 1,
      targetWidth: 300,
      resultOptions: {},
    }
    const fusionPlan: PreparedFusionExecutionPlan = {
      kind: 'fusion',
      sourceNodeId: 'fusion-1',
      source,
      taskPrompt: '[style-fusion] blend',
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

    expect(buildImageExecutionTaskRequest(actionPlan).feature).toBe(EXECUTION_FEATURE.removeBg)
    expect(buildImageExecutionTaskRequest(textPlan)).toMatchObject({
      feature: EXECUTION_FEATURE.promptGeneration,
    })
    expect(buildImageExecutionTaskRequest(textPlan)).not.toHaveProperty('image_urls')
    expect(buildImageExecutionTaskRequest(textPlan)).toMatchObject({ params: { output_count: 1 } })
    expect(buildImageExecutionTaskRequest(extendPlan)).toMatchObject({
      feature: EXECUTION_FEATURE.imageExtend,
      image_urls: [source.url],
      params: {
        extend: {
          right_percent: 10,
        },
      },
    })
    expect(buildImageExecutionTaskRequest(fusionPlan)).toMatchObject({
      feature: EXECUTION_FEATURE.styleFusion,
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
      kind: 'image-inpaint',
      sourceNodeId: 'image-1',
      source,
      prompt: 'replace object',
      rawMaskDataUrl: 'data:image/png;base64,mask',
      outputCount: 1,
      referenceImageUrls: [' https://assets.example.com/ref.png '],
      resultOptions: {},
    }

    expect(
      buildPreparedImageInpaintTaskRequest({
        plan,
        maskImageUrl: 'https://assets.example.com/mask.png',
      }),
    ).toMatchObject({
      feature: EXECUTION_FEATURE.localInpaint,
      params: {
        output_count: 1,
        original_image_url: source.url,
        mask_image_url: 'https://assets.example.com/mask.png',
        reference_image_urls: ['https://assets.example.com/ref.png'],
        use_mask: true,
      },
    })
  })
})
