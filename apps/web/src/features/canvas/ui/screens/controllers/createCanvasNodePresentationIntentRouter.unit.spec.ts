import { describe, expect, it, vi } from 'vitest'
import type { ImageNodeAction } from '../../../core/nodes/imageActionRegistry.ts'
import type { OutpaintDragAnchor } from '../../../tools/outpaint/session/useImageOutpaintOverlay.ts'
import type { CanvasNodePresentationIntent } from '../../nodes'
import type { FlowSnapshotParamPatch } from '../../../session/flow-adapter'
import { createCanvasNodePresentationIntentRouter } from './createCanvasNodePresentationIntentRouter'

type RoutingCase = {
  intent: CanvasNodePresentationIntent
  command: ReturnType<typeof vi.fn>
  args: unknown[]
}

function createCommandSurface() {
  return {
    prompt: {
      updateText: vi.fn(),
      updateParameters: vi.fn(),
      generate: vi.fn(),
    },
    node: { delete: vi.fn(), retryImageImport: vi.fn() },
    mediaInput: {
      drop: vi.fn(),
      open: vi.fn(),
      preview: vi.fn(),
      remove: vi.fn(),
    },
    image: {
      dropStage: vi.fn(),
      openFile: vi.fn(),
      preview: vi.fn(),
      appendToCanvas: vi.fn(),
      reset: vi.fn(),
      openContextMenu: vi.fn(),
      generateFromPrompt: vi.fn(),
      runAction: vi.fn(),
    },
    outpaint: {
      submit: vi.fn(),
      beginHandle: vi.fn(),
      moveHandle: vi.fn(),
      endHandle: vi.fn(),
      beginFrame: vi.fn(),
    },
    fusion: {
      openContextMenu: vi.fn(),
      generate: vi.fn(),
    },
  }
}

describe('Canvas Node presentation intent routing seam', () => {
  it('routes prompt, node, and media input intents', () => {
    const commands = createCommandSurface()
    const router = createCanvasNodePresentationIntentRouter(commands)
    const nodeId = 'node-1'
    const image = { url: 'https://example.com/input.png', ratio: 1 }
    const patch = {} as FlowSnapshotParamPatch
    const dragEvent = new DragEvent('drop')

    const cases: RoutingCase[] = [
      {
        intent: { type: 'text-edited', nodeId, text: 'draft' },
        command: commands.prompt.updateText,
        args: [nodeId, 'draft'],
      },
      {
        intent: { type: 'parameter-changed', nodeId, patch },
        command: commands.prompt.updateParameters,
        args: [nodeId, patch],
      },
      {
        intent: { type: 'delete-requested', nodeId },
        command: commands.node.delete,
        args: [nodeId],
      },
      {
        intent: { type: 'image-import-retry-requested', nodeId },
        command: commands.node.retryImageImport,
        args: [nodeId],
      },
      {
        intent: { type: 'media-input-dropped', nodeId, event: dragEvent },
        command: commands.mediaInput.drop,
        args: [nodeId, dragEvent],
      },
      {
        intent: { type: 'media-input-open-requested', nodeId },
        command: commands.mediaInput.open,
        args: [nodeId],
      },
      {
        intent: { type: 'media-input-preview-requested', nodeId, image },
        command: commands.mediaInput.preview,
        args: [image],
      },
      {
        intent: { type: 'media-input-remove-requested', nodeId, imageId: 'input-1' },
        command: commands.mediaInput.remove,
        args: [nodeId, 'input-1'],
      },
      {
        intent: {
          type: 'generation-requested',
          nodeId,
          prompt: 'make it',
          outputCount: 2,
          aspectRatio: '1:1',
        },
        command: commands.prompt.generate,
        args: [nodeId, 'make it', 2, '1:1'],
      },
    ]

    for (const { intent, command, args } of cases) {
      router.dispatch(intent)
      expect(command).toHaveBeenCalledWith(...args)
    }
  })

  it('routes image editing and display intents', () => {
    const commands = createCommandSurface()
    const router = createCanvasNodePresentationIntentRouter(commands)
    const nodeId = 'node-1'
    const action = {} as ImageNodeAction
    const dragEvent = new DragEvent('drop')
    const mouseEvent = new MouseEvent('contextmenu')

    const cases: RoutingCase[] = [
      {
        intent: { type: 'stage-dropped', nodeId, event: dragEvent },
        command: commands.image.dropStage,
        args: [nodeId, dragEvent],
      },
      {
        intent: { type: 'file-open-requested', nodeId },
        command: commands.image.openFile,
        args: [nodeId],
      },
      {
        intent: { type: 'preview-requested', nodeId },
        command: commands.image.preview,
        args: [nodeId],
      },
      {
        intent: { type: 'append-to-canvas-requested', nodeId },
        command: commands.image.appendToCanvas,
        args: [nodeId],
      },
      {
        intent: { type: 'reset-requested', nodeId },
        command: commands.image.reset,
        args: [nodeId],
      },
      {
        intent: { type: 'image-context-menu-requested', nodeId, event: mouseEvent },
        command: commands.image.openContextMenu,
        args: [nodeId, mouseEvent],
      },
      {
        intent: { type: 'action-requested', nodeId, action, prompt: '', outputCount: 1 },
        command: commands.image.runAction,
        args: [nodeId, action, '', 1, undefined],
      },
    ]

    for (const { intent, command, args } of cases) {
      router.dispatch(intent)
      expect(command).toHaveBeenCalledWith(...args)
    }
  })

  it('routes outpaint and fusion intents', () => {
    const commands = createCommandSurface()
    const router = createCanvasNodePresentationIntentRouter(commands)
    const nodeId = 'node-1'
    const anchor = {} as OutpaintDragAnchor
    const mouseEvent = new MouseEvent('contextmenu')
    const pointerEvent = new PointerEvent('pointerdown')

    const cases: RoutingCase[] = [
      {
        intent: { type: 'outpaint-submitted', nodeId, prompt: 'extend' },
        command: commands.outpaint.submit,
        args: [nodeId, 'extend'],
      },
      {
        intent: { type: 'outpaint-handle-pointerdown', nodeId, anchor, event: pointerEvent },
        command: commands.outpaint.beginHandle,
        args: [nodeId, anchor, pointerEvent],
      },
      {
        intent: { type: 'outpaint-handle-pointermove', nodeId, event: pointerEvent },
        command: commands.outpaint.moveHandle,
        args: [pointerEvent],
      },
      {
        intent: { type: 'outpaint-handle-pointerup', nodeId, event: pointerEvent },
        command: commands.outpaint.endHandle,
        args: [pointerEvent],
      },
      {
        intent: { type: 'outpaint-frame-pointerdown', nodeId, event: pointerEvent },
        command: commands.outpaint.beginFrame,
        args: [nodeId, pointerEvent],
      },
      {
        intent: { type: 'fusion-context-menu-requested', nodeId, event: mouseEvent },
        command: commands.fusion.openContextMenu,
        args: [nodeId, mouseEvent],
      },
      {
        intent: {
          type: 'fusion-generation-requested',
          nodeId,
          prompt: 'blend',
          outputCount: 4,
          aspectRatio: '16:9',
        },
        command: commands.fusion.generate,
        args: [nodeId, 'blend', 4, '16:9'],
      },
    ]

    for (const { intent, command, args } of cases) {
      router.dispatch(intent)
      expect(command).toHaveBeenCalledWith(...args)
    }
  })

  it('fails fast for an unsupported Node intent', () => {
    const router = createCanvasNodePresentationIntentRouter(createCommandSurface())

    expect(() => router.dispatch({ type: 'unsupported' } as never)).toThrow(
      'Unsupported Canvas Node presentation intent',
    )
  })
})
