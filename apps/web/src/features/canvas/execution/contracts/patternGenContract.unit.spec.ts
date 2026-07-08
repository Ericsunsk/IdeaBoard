import { describe, expect, it } from 'vitest'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'
import {
  PATTERN_GEN_NODE_IO_CONTRACT,
  assertPatternGenTaskRequest,
} from './patternGenContract'

describe('patternGenContract', () => {
  it('accepts the text prompt node IO request contract', () => {
    expect(PATTERN_GEN_NODE_IO_CONTRACT).toMatchObject({
      feature: WORKFLOW_FEATURE.patternGen,
      input: {
        prompt: { required: true },
        ratio: ['auto', '1:1', '3:4', '4:3', '9:16', '16:9'],
      },
      output: {
        kind: 'image-results',
        minCount: 1,
      },
    })

    expect(() =>
      assertPatternGenTaskRequest({
        prompt: 'repeatable textile pattern',
        feature: WORKFLOW_FEATURE.patternGen,
        params: { ratio: '1:1' },
      }),
    ).not.toThrow()
  })

  it('rejects invalid text prompt inputs before task submission', () => {
    expect(() =>
      assertPatternGenTaskRequest({
        prompt: 'repeatable textile pattern',
        feature: WORKFLOW_FEATURE.patternGen,
        params: { ratio: '2:1' },
      }),
    ).toThrow('图案生成尺寸比例仅支持 auto、1:1、3:4、4:3、9:16、16:9')

    expect(() =>
      assertPatternGenTaskRequest({
        prompt: 'repeatable textile pattern',
        feature: WORKFLOW_FEATURE.patternGen,
        image_urls: ['https://assets.example.com/source.png'],
      }),
    ).toThrow('图案生成不需要输入图片，请移除图片输入后重试')
  })
})
