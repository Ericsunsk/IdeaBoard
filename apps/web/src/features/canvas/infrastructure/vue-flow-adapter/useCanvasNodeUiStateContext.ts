import { hasInjectionContext, inject, provide, type InjectionKey } from 'vue'
import type { NodeUiState, PromptNodeUiState } from '../../domain/nodes/canvasNodeUi.ts'

export type CanvasNodeUiStateGetter = (nodeId: string) => NodeUiState | PromptNodeUiState

export type CanvasNodeUiStateContext = {
  getFlowNodeUiState: CanvasNodeUiStateGetter
}

const CANVAS_NODE_UI_STATE_CONTEXT_KEY: InjectionKey<CanvasNodeUiStateContext> =
  Symbol('canvas:node-ui-state')

const defaultCanvasNodeUiStateContext: CanvasNodeUiStateContext = {
  getFlowNodeUiState: () => ({
    connection: { hasAnyConnection: false, connected: {} },
  }),
}

export function provideCanvasNodeUiStateContext(context: CanvasNodeUiStateContext) {
  provide(CANVAS_NODE_UI_STATE_CONTEXT_KEY, context)
}

export function useCanvasNodeUiStateContext(): CanvasNodeUiStateContext {
  if (!hasInjectionContext()) return defaultCanvasNodeUiStateContext
  return inject(CANVAS_NODE_UI_STATE_CONTEXT_KEY, defaultCanvasNodeUiStateContext)
}
