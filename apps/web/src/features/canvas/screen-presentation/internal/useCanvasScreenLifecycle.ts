import { onBeforeUnmount, onMounted, watch, type Ref } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { CanvasConnectionPort } from '../../../canvas-board/domain'
import { useFlowKeyboardShortcuts } from '../../composables/useFlowKeyboardShortcuts'
import { useFlowLeaveGuard } from '../../composables/useFlowLeaveGuard'
import type { CanvasGraphNodeId } from '../../graph-runtime'
import type { CanvasWorkspaceRuntime } from '../../workspace-runtime'

type CanvasBenchmarkConnectDeleteTarget = {
  sourceNodeId: CanvasGraphNodeId
  targetNodeId: CanvasGraphNodeId
  sourcePort: CanvasConnectionPort
  targetPort: CanvasConnectionPort
  deleteNodeId: CanvasGraphNodeId
}

type CanvasBenchmarkWindow = Window & {
  __ideaboardCanvasBenchmarkActions?: {
    connectDelete: (target: CanvasBenchmarkConnectDeleteTarget) => void
  }
}

export function useCanvasScreenLifecycle(options: {
  route: RouteLocationNormalizedLoaded
  persistence: CanvasWorkspaceRuntime['commands']['persistence']
  history: CanvasWorkspaceRuntime['viewModel']['persistence']['history']
  duplicateModifierPressed: Ref<boolean>
  keyboard: {
    onDeleteInlineSelection: () => boolean
    isShortcutSuppressed: () => boolean
    onEscape: () => boolean
    onPanViewport: CanvasWorkspaceRuntime['commands']['viewportControls']['onPanViewport']
    setActiveTool: (tool: 'select' | 'hand') => void
  }
  benchmark: {
    connectIntent: CanvasWorkspaceRuntime['commands']['flowPane']['connectionAdapter']['connectIntent']
    onNodesChange: CanvasWorkspaceRuntime['commands']['flowPane']['onNodesChange']
  }
}) {
  async function loadBoardSession() {
    await options.persistence.recoverBoardSession()
  }

  watch(() => options.route.query.boardId, loadBoardSession)

  useFlowKeyboardShortcuts({
    onSave: () => {
      void options.persistence.flushAutosave({ silent: false })
    },
    onUndo: options.history.undo,
    onRedo: options.history.redo,
    onDeleteInlineSelection: options.keyboard.onDeleteInlineSelection,
    isCanvasShortcutSuppressed: options.keyboard.isShortcutSuppressed,
    onEscape: options.keyboard.onEscape,
    onPanViewport: options.keyboard.onPanViewport,
    onToolSelect: () => options.keyboard.setActiveTool('select'),
    onToolHand: () => options.keyboard.setActiveTool('hand'),
    onDuplicateModifierChange: (pressed) => {
      options.duplicateModifierPressed.value = pressed
    },
  })

  useFlowLeaveGuard({
    onLeave: () => options.persistence.flushAutosave(),
  })

  if (import.meta.env.DEV && options.route.query.canvasBenchmarkActions === '1') {
    const benchmarkWindow = window as CanvasBenchmarkWindow
    benchmarkWindow.__ideaboardCanvasBenchmarkActions = {
      connectDelete: (target) => {
        options.benchmark.connectIntent({
          sourceNodeId: target.sourceNodeId,
          targetNodeId: target.targetNodeId,
          sourcePort: target.sourcePort,
          targetPort: target.targetPort,
        })
        options.benchmark.onNodesChange([{ id: target.deleteNodeId, type: 'remove' }])
      },
    }
    onBeforeUnmount(() => {
      delete benchmarkWindow.__ideaboardCanvasBenchmarkActions
    })
  }

  onMounted(loadBoardSession)
}
