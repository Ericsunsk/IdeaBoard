import type { CanvasNodePresentationIntent } from '../node-presentation'

type CanvasNodeIntentOf<Type extends CanvasNodePresentationIntent['type']> = Extract<
  CanvasNodePresentationIntent,
  { type: Type }
>

export type CanvasNodePresentationIntentCommands = {
  prompt: {
    updateText: (nodeId: string, text: string) => void
    updateParameters: (
      nodeId: string,
      patch: CanvasNodeIntentOf<'parameter-changed'>['patch'],
    ) => void
    generate: (
      nodeId: string,
      prompt: string,
      outputCount: number,
      aspectRatio: CanvasNodeIntentOf<'prompt-generation-requested'>['aspectRatio'],
    ) => void
  }
  node: {
    delete: (nodeId: string) => void
  }
  mediaInput: {
    drop: (nodeId: string, event: DragEvent) => void
    open: (nodeId: string) => void
    preview: (image: CanvasNodeIntentOf<'media-input-preview-requested'>['image']) => void
    remove: (nodeId: string, imageId: string) => void
  }
  image: {
    dropStage: (nodeId: string, event: DragEvent) => void
    openFile: (nodeId: string) => void
    preview: (nodeId: string) => void
    appendToCanvas: (nodeId: string) => void
    reset: (nodeId: string) => void
    openContextMenu: (nodeId: string, event: MouseEvent) => void
    runAction: (
      nodeId: string,
      action: CanvasNodeIntentOf<'action-requested'>['action'],
      prompt: string,
      outputCount: number,
      strength?: number,
    ) => void
  }
  canvasExtend: {
    submit: (nodeId: string, prompt: string, outputCount: number) => void
    beginHandle: (
      nodeId: string,
      anchor: CanvasNodeIntentOf<'canvas-extend-handle-pointerdown'>['anchor'],
      event: PointerEvent,
    ) => void
    moveHandle: (event: PointerEvent) => void
    endHandle: (event: PointerEvent) => void
    beginFrame: (nodeId: string, event: PointerEvent) => void
  }
  fusion: {
    openContextMenu: (nodeId: string, event: MouseEvent) => void
    generate: (
      nodeId: string,
      prompt: string,
      outputCount: number,
      aspectRatio: CanvasNodeIntentOf<'fusion-generation-requested'>['aspectRatio'],
    ) => void
  }
}

export type CanvasNodePresentationIntentRouter = {
  dispatch: (intent: CanvasNodePresentationIntent) => void
}

export function createCanvasNodePresentationIntentRouter(
  commands: CanvasNodePresentationIntentCommands,
): CanvasNodePresentationIntentRouter {
  function dispatch(intent: CanvasNodePresentationIntent) {
    switch (intent.type) {
      case 'text-edited':
        commands.prompt.updateText(intent.nodeId, intent.text)
        return
      case 'parameter-changed':
        commands.prompt.updateParameters(intent.nodeId, intent.patch)
        return
      case 'delete-requested':
        commands.node.delete(intent.nodeId)
        return
      case 'media-input-dropped':
        commands.mediaInput.drop(intent.nodeId, intent.event)
        return
      case 'media-input-open-requested':
        commands.mediaInput.open(intent.nodeId)
        return
      case 'media-input-preview-requested':
        commands.mediaInput.preview(intent.image)
        return
      case 'media-input-remove-requested':
        commands.mediaInput.remove(intent.nodeId, intent.imageId)
        return
      case 'prompt-generation-requested':
        commands.prompt.generate(
          intent.nodeId,
          intent.prompt,
          intent.outputCount,
          intent.aspectRatio,
        )
        return
      case 'stage-dropped':
        commands.image.dropStage(intent.nodeId, intent.event)
        return
      case 'file-open-requested':
        commands.image.openFile(intent.nodeId)
        return
      case 'preview-requested':
        commands.image.preview(intent.nodeId)
        return
      case 'append-to-canvas-requested':
        commands.image.appendToCanvas(intent.nodeId)
        return
      case 'reset-requested':
        commands.image.reset(intent.nodeId)
        return
      case 'image-context-menu-requested':
        commands.image.openContextMenu(intent.nodeId, intent.event)
        return
      case 'canvas-extend-submitted':
        commands.canvasExtend.submit(intent.nodeId, intent.prompt, intent.outputCount)
        return
      case 'action-requested':
        commands.image.runAction(
          intent.nodeId,
          intent.action,
          intent.prompt,
          intent.outputCount,
          intent.strength,
        )
        return
      case 'canvas-extend-handle-pointerdown':
        commands.canvasExtend.beginHandle(intent.nodeId, intent.anchor, intent.event)
        return
      case 'canvas-extend-handle-pointermove':
        commands.canvasExtend.moveHandle(intent.event)
        return
      case 'canvas-extend-handle-pointerup':
        commands.canvasExtend.endHandle(intent.event)
        return
      case 'canvas-extend-frame-pointerdown':
        commands.canvasExtend.beginFrame(intent.nodeId, intent.event)
        return
      case 'fusion-context-menu-requested':
        commands.fusion.openContextMenu(intent.nodeId, intent.event)
        return
      case 'fusion-generation-requested':
        commands.fusion.generate(
          intent.nodeId,
          intent.prompt,
          intent.outputCount,
          intent.aspectRatio,
        )
        return
      default:
        return assertUnreachable(intent)
    }
  }

  return { dispatch }
}

function assertUnreachable(value: never): never {
  throw new Error(`Unsupported Canvas Node presentation intent: ${JSON.stringify(value)}`)
}
