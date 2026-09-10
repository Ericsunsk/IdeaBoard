import type { CanvasConnectionNodeType, WorkflowNodeOutputStatus } from '../../board/domain'

export type NodePrimaryActionState = {
  label: string
  disabled: boolean
}

export type NodeConnectionMap<TPort extends string = string> = Partial<
  Record<TPort, boolean | undefined>
>
export type NodeConnectionSourceMap<TPort extends string = string> = Partial<
  Record<TPort, CanvasConnectionNodeType | undefined>
>

export type NodeConnectionState<TPort extends string = string> = {
  hasAnyConnection?: boolean
  connected?: NodeConnectionMap<TPort>
  pending?: NodeConnectionMap<TPort>
  sourceKind?: NodeConnectionSourceMap<TPort>
}

export type NodeUiState<TPort extends string = string> = {
  connection: NodeConnectionState<TPort>
  primaryAction?: NodePrimaryActionState
  cacheStatus?: WorkflowNodeOutputStatus
  isRunning?: boolean
}

export type PromptNodeUiState = NodeUiState<'output'>
