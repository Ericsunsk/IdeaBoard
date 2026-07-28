import type { ImageNodePresentationIntent } from './imageNodePresentation'
import type { FusionNodePresentationIntent } from './fusionNodePresentation'
import type { PromptNodePresentationIntent } from './promptNodePresentation'

export type CanvasNodePresentationIntent =
  | ({ nodeKind: 'prompt' } & PromptNodePresentationIntent)
  | ({ nodeKind: 'image' } & ImageNodePresentationIntent)
  | ({ nodeKind: 'fusion' } & FusionNodePresentationIntent)

export type CanvasNodePresentationIntentHandlers = {
  prompt: (intent: PromptNodePresentationIntent) => void
  image: (intent: ImageNodePresentationIntent) => void
  fusion: (intent: FusionNodePresentationIntent) => void
}

export function createCanvasNodePresentationIntentHandler(
  handlers: CanvasNodePresentationIntentHandlers,
) {
  return (intent: CanvasNodePresentationIntent) => {
    switch (intent.nodeKind) {
      case 'prompt':
        handlers.prompt(intent)
        return
      case 'image':
        handlers.image(intent)
        return
      case 'fusion':
        handlers.fusion(intent)
        return
      default:
        return assertUnreachable(intent)
    }
  }
}

function assertUnreachable(value: never): never {
  throw new Error(`Unsupported Canvas Node presentation intent: ${JSON.stringify(value)}`)
}
