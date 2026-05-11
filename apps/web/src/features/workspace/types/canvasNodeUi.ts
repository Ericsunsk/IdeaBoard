import type { WorkflowNodeOutputStatus } from './canvasNodes'

export type NodePrimaryActionState = {
  label: string
  disabled: boolean
}

export type NodeConnectionMap<TPort extends string = string> = Partial<
  Record<TPort, boolean | undefined>
>

export type NodeConnectionState<TPort extends string = string> = {
  hasAnyConnection?: boolean
  connected?: NodeConnectionMap<TPort>
  pending?: NodeConnectionMap<TPort>
}

export type NodeUiState<TPort extends string = string> = {
  connection: NodeConnectionState<TPort>
  primaryAction?: NodePrimaryActionState
  cacheStatus?: WorkflowNodeOutputStatus
  isRunning?: boolean
}

export type PromptNodeUiState = NodeUiState<'output'>
