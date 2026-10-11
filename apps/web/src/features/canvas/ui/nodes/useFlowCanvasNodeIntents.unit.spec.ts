import { describe, expect, it } from 'vitest'
import type { CanvasNodePresentationIntent } from '.'
import {
  useFlowCanvasNodeIntents,
  type FlowCanvasNodeIntentEmitter,
} from './useFlowCanvasNodeIntents'

function createHarness(promptSnapshot: { text: string } | null = null) {
  const intents: CanvasNodePresentationIntent[] = []
  const mousedowns: Array<{ nodeId: string; event: MouseEvent }> = []
  const emit: FlowCanvasNodeIntentEmitter = {
    nodeIntent: (intent) => {
      intents.push(intent)
    },
    nodeMousedown: (nodeId, event) => {
      mousedowns.push({ nodeId, event })
    },
  }
  const intentsApi = useFlowCanvasNodeIntents({
    nodeId: () => 'image-1',
    promptSnapshot: () => promptSnapshot,
    emit,
  })
  return { intents, mousedowns, intentsApi }
}

describe('useFlowCanvasNodeIntents', () => {
  it('forwards text, parameter, and delete intents with the node id', () => {
    const { intents, intentsApi } = createHarness()

    intentsApi.onPromptTextEdited('hello')
    intentsApi.onPromptParameterChanged('title', 'T')
    intentsApi.onImageDeleteRequest()
    intentsApi.onPromptDeleteRequest()

    expect(intents).toEqual([
      { type: 'text-edited', nodeId: 'image-1', text: 'hello' },
      {
        type: 'parameter-changed',
        nodeId: 'image-1',
        patch: { key: 'title', value: 'T' },
      },
      { type: 'delete-requested', nodeId: 'image-1' },
      { type: 'delete-requested', nodeId: 'image-1' },
    ])
  })

  it('forwards mousedown with the node id and event', () => {
    const { mousedowns, intentsApi } = createHarness()
    const event = new MouseEvent('mousedown')

    intentsApi.onNodeMouseDown(event)

    expect(mousedowns).toEqual([{ nodeId: 'image-1', event }])
  })

  it('skips generation without a prompt snapshot and defaults missing fields', () => {
    const withoutSnapshot = createHarness(null)
    withoutSnapshot.intentsApi.onPromptGenerate()
    expect(withoutSnapshot.intents).toEqual([])

    const withSnapshot = createHarness({ text: 'draw a cat' })
    withSnapshot.intentsApi.onPromptGenerate()
    expect(withSnapshot.intents).toEqual([
      {
        type: 'generation-requested',
        nodeId: 'image-1',
        prompt: 'draw a cat',
        outputCount: 1,
        aspectRatio: 'auto',
      },
    ])
  })

  it('forwards media input and outpaint intents', () => {
    const { intents, intentsApi } = createHarness()
    const drop = new DragEvent('drop')

    intentsApi.onPromptMediaInputDrop(drop)
    intentsApi.onPromptMediaInputRemove({ id: 'external:1' })
    intentsApi.onImageOutpaintSubmit('expand sky')

    expect(intents).toEqual([
      { type: 'media-input-dropped', nodeId: 'image-1', event: drop },
      { type: 'media-input-remove-requested', nodeId: 'image-1', imageId: 'external:1' },
      { type: 'outpaint-submitted', nodeId: 'image-1', prompt: 'expand sky' },
    ])
  })
})
