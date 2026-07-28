import { describe, expect, it, vi } from 'vitest'
import { createPromptNodePresentationIntentHandler } from './index'

describe('Prompt Node presentation seam', () => {
  it('translates a text editing intent into the Canvas command', () => {
    const onTextEdited = vi.fn()
    const handleIntent = createPromptNodePresentationIntentHandler({
      onTextEdited,
      onParameterChanged: vi.fn(),
      onDeleteRequested: vi.fn(),
      onMediaInputDrop: vi.fn(),
      onMediaInputOpen: vi.fn(),
      onMediaInputPreview: vi.fn(),
      onMediaInputRemove: vi.fn(),
      onGenerationRequested: vi.fn(),
    })

    handleIntent({ type: 'text-edited', nodeId: 'prompt-1', text: '新的提示词' })

    expect(onTextEdited).toHaveBeenCalledWith('prompt-1', '新的提示词')
  })

  it('keeps parameter, generation, and deletion intents at the presentation boundary', () => {
    const onParameterChanged = vi.fn()
    const onDeleteRequested = vi.fn()
    const onGenerationRequested = vi.fn()
    const handleIntent = createPromptNodePresentationIntentHandler({
      onTextEdited: vi.fn(),
      onParameterChanged,
      onDeleteRequested,
      onMediaInputDrop: vi.fn(),
      onMediaInputOpen: vi.fn(),
      onMediaInputPreview: vi.fn(),
      onMediaInputRemove: vi.fn(),
      onGenerationRequested,
    })

    handleIntent({
      type: 'parameter-changed',
      nodeId: 'prompt-1',
      patch: { key: 'outputCount', value: 2 },
    })
    handleIntent({ type: 'delete-requested', nodeId: 'prompt-1' })
    handleIntent({
      type: 'generation-requested',
      nodeId: 'prompt-1',
      prompt: '新的提示词',
      outputCount: 2,
      aspectRatio: 'square',
    })

    expect(onParameterChanged).toHaveBeenCalledWith('prompt-1', {
      key: 'outputCount',
      value: 2,
    })
    expect(onDeleteRequested).toHaveBeenCalledWith('prompt-1')
    expect(onGenerationRequested).toHaveBeenCalledWith('prompt-1', '新的提示词', 2, 'square')
  })
})
