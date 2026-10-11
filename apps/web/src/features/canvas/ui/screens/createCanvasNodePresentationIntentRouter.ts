import type { CanvasNodePresentationIntent } from '../nodes'

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
      aspectRatio: CanvasNodeIntentOf<'generation-requested'>['aspectRatio'],
    ) => void
  }
  node: {
    delete: (nodeId: string) => void
    retryImageImport: (nodeId: string) => void
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
  outpaint: {
    submit: (nodeId: string, prompt: string) => void
    beginHandle: (
      nodeId: string,
      anchor: CanvasNodeIntentOf<'outpaint-handle-pointerdown'>['anchor'],
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

type IntentHandlerMap = {
  [Type in CanvasNodePresentationIntent['type']]: (
    commands: CanvasNodePresentationIntentCommands,
    intent: CanvasNodeIntentOf<Type>,
  ) => void
}

const INTENT_HANDLERS: IntentHandlerMap = {
  'text-edited': (commands, intent) => {
    commands.prompt.updateText(intent.nodeId, intent.text)
  },
  'parameter-changed': (commands, intent) => {
    commands.prompt.updateParameters(intent.nodeId, intent.patch)
  },
  'delete-requested': (commands, intent) => {
    commands.node.delete(intent.nodeId)
  },
  'image-import-retry-requested': (commands, intent) => {
    commands.node.retryImageImport(intent.nodeId)
  },
  'media-input-dropped': (commands, intent) => {
    commands.mediaInput.drop(intent.nodeId, intent.event)
  },
  'media-input-open-requested': (commands, intent) => {
    commands.mediaInput.open(intent.nodeId)
  },
  'media-input-preview-requested': (commands, intent) => {
    commands.mediaInput.preview(intent.image)
  },
  'media-input-remove-requested': (commands, intent) => {
    commands.mediaInput.remove(intent.nodeId, intent.imageId)
  },
  'generation-requested': (commands, intent) => {
    commands.prompt.generate(intent.nodeId, intent.prompt, intent.outputCount, intent.aspectRatio)
  },
  'stage-dropped': (commands, intent) => {
    commands.image.dropStage(intent.nodeId, intent.event)
  },
  'file-open-requested': (commands, intent) => {
    commands.image.openFile(intent.nodeId)
  },
  'preview-requested': (commands, intent) => {
    commands.image.preview(intent.nodeId)
  },
  'append-to-canvas-requested': (commands, intent) => {
    commands.image.appendToCanvas(intent.nodeId)
  },
  'reset-requested': (commands, intent) => {
    commands.image.reset(intent.nodeId)
  },
  'image-context-menu-requested': (commands, intent) => {
    commands.image.openContextMenu(intent.nodeId, intent.event)
  },
  'outpaint-submitted': (commands, intent) => {
    commands.outpaint.submit(intent.nodeId, intent.prompt)
  },
  'action-requested': (commands, intent) => {
    commands.image.runAction(
      intent.nodeId,
      intent.action,
      intent.prompt,
      intent.outputCount,
      intent.strength,
    )
  },
  'outpaint-handle-pointerdown': (commands, intent) => {
    commands.outpaint.beginHandle(intent.nodeId, intent.anchor, intent.event)
  },
  'outpaint-handle-pointermove': (commands, intent) => {
    commands.outpaint.moveHandle(intent.event)
  },
  'outpaint-handle-pointerup': (commands, intent) => {
    commands.outpaint.endHandle(intent.event)
  },
  'outpaint-frame-pointerdown': (commands, intent) => {
    commands.outpaint.beginFrame(intent.nodeId, intent.event)
  },
  'fusion-context-menu-requested': (commands, intent) => {
    commands.fusion.openContextMenu(intent.nodeId, intent.event)
  },
  'fusion-generation-requested': (commands, intent) => {
    commands.fusion.generate(intent.nodeId, intent.prompt, intent.outputCount, intent.aspectRatio)
  },
}

export function createCanvasNodePresentationIntentRouter(
  commands: CanvasNodePresentationIntentCommands,
): CanvasNodePresentationIntentRouter {
  function dispatch(intent: CanvasNodePresentationIntent) {
    const handler = INTENT_HANDLERS[intent.type] as
      | ((
          commands: CanvasNodePresentationIntentCommands,
          intent: CanvasNodePresentationIntent,
        ) => void)
      | undefined
    if (!handler) {
      throw new Error(`Unsupported Canvas Node presentation intent: ${JSON.stringify(intent)}`)
    }
    handler(commands, intent)
  }

  return { dispatch }
}
