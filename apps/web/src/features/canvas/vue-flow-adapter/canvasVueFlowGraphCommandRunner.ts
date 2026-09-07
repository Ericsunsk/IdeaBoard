import type { Edge, Node } from '@vue-flow/core'
import { recordCanvasGraphCommand } from '../utils/canvasInteractionMetrics'

export type CanvasVueFlowGraph = {
  nodes: Node[]
  edges: Edge[]
}

export type CanvasVueFlowGraphCommandName =
  | 'replaceGraph'
  | 'replaceNodes'
  | 'replaceEdges'
  | 'addNodes'
  | 'addEdges'
  | 'removeNodes'
  | 'removeEdges'
  | 'updateNode'
  | 'applyGraphChanges'
  | 'applyNodeChanges'
  | 'applyEdgeChanges'

export type CanvasVueFlowGraphCommandContext = {
  command: CanvasVueFlowGraphCommandName
  before: CanvasVueFlowGraph
  next: CanvasVueFlowGraph
  metadata?: Record<string, unknown> | undefined
}

export type CanvasVueFlowGraphCommandResult = {
  command: CanvasVueFlowGraphCommandName
  before: CanvasVueFlowGraph
  after: CanvasVueFlowGraph
  didChange: boolean
  metadata?: Record<string, unknown> | undefined
}

type BeforeCommandHook = (context: CanvasVueFlowGraphCommandContext) => unknown
type AfterCommandHook = (result: CanvasVueFlowGraphCommandResult, beforeHookValue: unknown) => void

export type CanvasVueFlowGraphCommandOptions = {
  metadata?: Record<string, unknown> | undefined
  beforeCommand?: BeforeCommandHook | undefined
  afterCommand?: AfterCommandHook | undefined
}

type GraphCommandRunnerOptions = Pick<
  CanvasVueFlowGraphCommandOptions,
  'beforeCommand' | 'afterCommand'
> & {
  readGraph: () => CanvasVueFlowGraph
}

type RunCommandParams = {
  command: CanvasVueFlowGraphCommandName
  nextGraph: CanvasVueFlowGraph
  commit: () => void
  // Normalized with ?? {} in run(); explicit undefined is equivalent to absent.
  commandOptions?: CanvasVueFlowGraphCommandOptions | undefined
}

export type CanvasVueFlowGraphCommandRunner = {
  noOp: (
    command: CanvasVueFlowGraphCommandName,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ) => CanvasVueFlowGraphCommandResult
  run: (params: RunCommandParams) => CanvasVueFlowGraphCommandResult
  runNative: (
    command: CanvasVueFlowGraphCommandName,
    commit: () => void,
    commandOptions?: CanvasVueFlowGraphCommandOptions,
  ) => CanvasVueFlowGraphCommandResult
}

function isSameGraph(left: CanvasVueFlowGraph, right: CanvasVueFlowGraph): boolean {
  return left.nodes === right.nodes && left.edges === right.edges
}

function createGraphCommandResult(
  command: CanvasVueFlowGraphCommandName,
  before: CanvasVueFlowGraph,
  after: CanvasVueFlowGraph,
  didChange: boolean,
  metadata: Record<string, unknown> | undefined,
): CanvasVueFlowGraphCommandResult {
  return {
    command,
    before,
    after,
    didChange,
    metadata,
  }
}

export function createGraphCommandRunner(
  options: GraphCommandRunnerOptions,
): CanvasVueFlowGraphCommandRunner {
  function noOp(
    command: CanvasVueFlowGraphCommandName,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const before = options.readGraph()
    return createGraphCommandResult(command, before, before, false, commandOptions.metadata)
  }

  function runHooks(
    context: CanvasVueFlowGraphCommandContext,
    resultFactory: () => CanvasVueFlowGraphCommandResult,
    commandOptions: CanvasVueFlowGraphCommandOptions,
  ): CanvasVueFlowGraphCommandResult {
    const globalBeforeValue = options.beforeCommand?.(context)
    const commandBeforeValue = commandOptions.beforeCommand?.(context)
    const result = resultFactory()
    options.afterCommand?.(result, globalBeforeValue)
    commandOptions.afterCommand?.(result, commandBeforeValue)
    return result
  }

  function runRecordedCommand(
    command: CanvasVueFlowGraphCommandName,
    before: CanvasVueFlowGraph,
    commit: () => void,
    metadata: Record<string, unknown> | undefined,
  ): CanvasVueFlowGraphCommandResult {
    commit()
    recordCanvasGraphCommand(command)
    return createGraphCommandResult(command, before, options.readGraph(), true, metadata)
  }

  function run(params: RunCommandParams): CanvasVueFlowGraphCommandResult {
    const commandOptions = params.commandOptions ?? {}
    const before = options.readGraph()
    if (isSameGraph(before, params.nextGraph)) return noOp(params.command, commandOptions)

    return runHooks(
      {
        command: params.command,
        before,
        next: params.nextGraph,
        metadata: commandOptions.metadata,
      },
      () => runRecordedCommand(params.command, before, params.commit, commandOptions.metadata),
      commandOptions,
    )
  }

  function runNative(
    command: CanvasVueFlowGraphCommandName,
    commit: () => void,
    commandOptions: CanvasVueFlowGraphCommandOptions = {},
  ): CanvasVueFlowGraphCommandResult {
    const before = options.readGraph()
    return runHooks(
      {
        command,
        before,
        next: before,
        metadata: commandOptions.metadata,
      },
      () => runRecordedCommand(command, before, commit, commandOptions.metadata),
      commandOptions,
    )
  }

  return {
    noOp,
    run,
    runNative,
  }
}
