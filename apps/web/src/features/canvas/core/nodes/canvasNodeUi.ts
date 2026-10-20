import type {
  CanvasConnectionNodeKind,
  WorkflowNodeOutputStatus,
} from '../../../board/ports/canvas'

type NodeConnectionMap<TPort extends string = string> = Partial<Record<TPort, boolean | undefined>>
type NodeConnectionSourceMap<TPort extends string = string> = Partial<
  Record<TPort, CanvasConnectionNodeKind | undefined>
>

type NodeConnectionState<TPort extends string = string> = {
  hasAnyConnection?: boolean
  connected?: NodeConnectionMap<TPort>
  pending?: NodeConnectionMap<TPort>
  sourceKind?: NodeConnectionSourceMap<TPort>
}

export type NodeUiState<TPort extends string = string> = {
  connection: NodeConnectionState<TPort>
  cacheStatus?: WorkflowNodeOutputStatus
  isRunning?: boolean
}

export type PromptNodeUiState = NodeUiState<'output'>
