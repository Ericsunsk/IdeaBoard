import { describe, expect, it, vi } from 'vitest'
import { createImageExecutionPlanner, createImageWorkflowCommands } from '../execution'
import { createGenerationIntent, type GenerationIntentOptions } from './generationIntent'

type MediaInputImage = ReturnType<GenerationIntentOptions['getMediaInputImages']>[number]

function createGenerationHarness(inputImages: MediaInputImage[]) {
  const onWarning = vi.fn()
  const execute = vi.fn()
  const commands = createImageWorkflowCommands({
    planner: createImageExecutionPlanner(() => ({ nodes: [], edges: [] })),
    workflow: {
      executeImageWorkflow: async (plan) => {
        execute(plan)
      },
      isNodeExecutionActive: () => false,
      reattachPendingImageResults: vi.fn(),
    },
    onWarning,
  })
  const intent = createGenerationIntent({
    cancelFlowOutpaintMode: vi.fn(),
    cancelFlowInpaintMode: vi.fn(),
    getMediaInputImages: () => inputImages,
    runGeneration: commands.runGeneration,
    onWarning,
  })

  return { intent, execute, onWarning }
}

const uploadedReference: MediaInputImage = {
  id: 'external:1',
  sourceType: 'external',
  sourceId: 'asset_uploaded',
  url: '/api/media/assets/asset_uploaded/preview',
  ratio: 0.75,
  mediaRef: { assetId: 'asset_uploaded', derivativeTier: 'preview' },
}

describe('generationIntent', () => {
  it.each(['external', 'node'] as const)(
    'preserves the asset ID of a managed %s Prompt reference through Apply',
    async (sourceType) => {
      const reference: MediaInputImage =
        sourceType === 'external'
          ? uploadedReference
          : {
              ...uploadedReference,
              id: 'node:image-1',
              sourceType,
              sourceId: 'image-1',
              sourceNodeId: 'image-1',
            }
      const { intent, execute, onWarning } = createGenerationHarness([reference])

      intent.onFlowPromptGenerate('prompt-1', 'use this reference', 1, 'auto')
      await vi.waitFor(() => expect(execute).toHaveBeenCalled())
      expect(onWarning).not.toHaveBeenCalled()
      expect(execute).toHaveBeenCalledWith(
        expect.objectContaining({
          referenceImages: [expect.objectContaining({ assetId: 'asset_uploaded' })],
        }),
      )
    },
  )

  it('preserves reference order and output count through Apply', async () => {
    const secondReference: MediaInputImage = {
      ...uploadedReference,
      id: 'node:image-2',
      sourceType: 'node',
      sourceId: 'image-2',
      sourceNodeId: 'image-2',
      mediaRef: { assetId: 'asset_generated', derivativeTier: 'large-preview' },
    }
    const { intent, execute, onWarning } = createGenerationHarness([
      secondReference,
      uploadedReference,
    ])

    intent.onFlowPromptGenerate('prompt-1', 'use both references', 3, 'auto')

    await vi.waitFor(() => expect(execute).toHaveBeenCalled())
    expect(execute).toHaveBeenCalledWith(
      expect.objectContaining({
        referenceImages: [
          expect.objectContaining({ assetId: 'asset_generated' }),
          expect.objectContaining({ assetId: 'asset_uploaded' }),
        ],
        outputCount: 3,
      }),
    )
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('preserves unbacked references for submission validation instead of dropping them', async () => {
    const { intent, execute, onWarning } = createGenerationHarness([
      uploadedReference,
      { ...uploadedReference, id: 'external:pending', url: 'blob:pending', mediaRef: undefined },
    ])

    intent.onFlowPromptGenerate('prompt-1', 'use both references', 1, 'auto')

    await vi.waitFor(() => expect(execute).toHaveBeenCalled())
    expect(execute).toHaveBeenCalledWith(
      expect.objectContaining({
        referenceImages: [
          expect.objectContaining({ assetId: 'asset_uploaded' }),
          { url: 'blob:pending', ratio: uploadedReference.ratio },
        ],
      }),
    )
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('keeps text-only generation available', async () => {
    const { intent, execute, onWarning } = createGenerationHarness([])

    intent.onFlowPromptGenerate('prompt-1', 'create a landscape', 1, 'auto')

    await vi.waitFor(() => expect(execute).toHaveBeenCalled())
    expect(execute).toHaveBeenCalledWith(expect.objectContaining({ prompt: 'create a landscape' }))
    expect(onWarning).not.toHaveBeenCalled()
  })
})
