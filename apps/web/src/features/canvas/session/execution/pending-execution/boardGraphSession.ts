import { resolveImageNodeFrameSize, type ImageNodeItem } from '../../../../board/ports/canvas'
import { CanvasGraphSession, type CanvasGraphStore } from '../../../core/graph'
import type { BoardAttachment, BoardSessionIdentity } from '../board-attachment'
import type { MediaRef } from '../../../../../core/media/mediaRef'

export type PendingImageResultNodeId = `image-${number}`

export type PendingImageResolvedResult = Readonly<{
  url: string
  mediaRef?: MediaRef
  ratio: number
  width?: number
  height?: number
  mimeType?: string
  resultIndex?: number
}>

export type PendingImageMutationRejectReason = 'missing-node' | 'non-image-node' | 'stale-task'

export type PendingImageDomainMutationInput = Readonly<{
  resultNodeId: PendingImageResultNodeId
  taskId: string
  result: PendingImageResolvedResult
}>

export type PendingImageDomainMutationOutcome =
  | Readonly<{ resultNodeId: PendingImageResultNodeId; kind: 'materialized' }>
  | Readonly<{
      resultNodeId: PendingImageResultNodeId
      kind: 'rejected'
      reason: PendingImageMutationRejectReason
    }>

export type PendingImageDomainMutationResult = Readonly<{
  didChange: boolean
  changedNodeIds: readonly PendingImageResultNodeId[]
  outcomes: readonly PendingImageDomainMutationOutcome[]
}>

export type PendingImageDomainMutationPort = Readonly<{
  attachment: BoardAttachment
  sessionIdentity: BoardSessionIdentity
  materializeBatch: (
    inputs: readonly PendingImageDomainMutationInput[],
  ) => PendingImageDomainMutationResult
}>

export type BoardGraphSessionBinding = Readonly<{
  attachment: BoardAttachment
  sessionIdentity: BoardSessionIdentity
  mutations: PendingImageDomainMutationPort
}>

export type BoardGraphSessionOptions = Readonly<{
  graphSession: CanvasGraphSession
  onMaterialized?:
    ((store: CanvasGraphStore, result: PendingImageDomainMutationResult) => void) | undefined
}>

export const bindBoardGraphSession = Symbol('bindBoardGraphSession')

function normalizeResolvedImageResult(
  result: PendingImageResolvedResult,
  currentSnapshot: ImageNodeItem,
): { ratio: number; width: number } {
  const resolvedWidth = Number(result.width)
  const resolvedHeight = Number(result.height)
  const hasResolvedSize =
    Number.isFinite(resolvedWidth) &&
    resolvedWidth > 0 &&
    Number.isFinite(resolvedHeight) &&
    resolvedHeight > 0
  const ratio = hasResolvedSize
    ? resolvedHeight / resolvedWidth
    : result.ratio > 0
      ? result.ratio
      : 1
  const currentWidth = Number(currentSnapshot.width)
  const width = hasResolvedSize
    ? resolveImageNodeFrameSize(ratio, true).width
    : Number.isFinite(currentWidth) && currentWidth > 0
      ? Math.max(1, Math.round(currentWidth))
      : resolveImageNodeFrameSize(ratio, true).width
  return { ratio, width }
}

function requireBoardId(boardId: string): string {
  const normalized = boardId.trim()
  if (!normalized) throw new Error('Canvas Board Graph Session boardId is required')
  return normalized
}

export class BoardGraphSession {
  readonly boardId: string
  readonly #graphSession: CanvasGraphSession
  readonly #onMaterialized?: BoardGraphSessionOptions['onMaterialized']
  readonly #identity = Object.freeze({}) as BoardSessionIdentity

  private constructor(boardId: string, options: BoardGraphSessionOptions) {
    this.boardId = requireBoardId(boardId)
    this.#graphSession = options.graphSession
    this.#onMaterialized = options.onMaterialized
    Object.freeze(this)
  }

  static create(boardId: string, options: BoardGraphSessionOptions): BoardGraphSession {
    return new BoardGraphSession(boardId, options)
  }

  [bindBoardGraphSession](epoch: number): BoardGraphSessionBinding {
    const attachment = Object.freeze({
      boardId: this.boardId,
      epoch,
      sessionIdentity: this.#identity,
    }) as BoardAttachment
    const mutations: PendingImageDomainMutationPort = Object.freeze({
      attachment,
      sessionIdentity: this.#identity,
      materializeBatch: (inputs) => this.#materializeBatch(inputs),
    })
    return Object.freeze({
      attachment,
      sessionIdentity: this.#identity,
      mutations,
    })
  }

  #materializeBatch(
    inputs: readonly PendingImageDomainMutationInput[],
  ): PendingImageDomainMutationResult {
    const store = this.#graphSession.store
    const transaction = store.beginTransaction('result-materialize', {
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    const outcomes: PendingImageDomainMutationOutcome[] = []

    for (const input of inputs) {
      const node = store.getNode(input.resultNodeId)
      if (!node) {
        outcomes.push({
          resultNodeId: input.resultNodeId,
          kind: 'rejected',
          reason: 'missing-node',
        })
        continue
      }
      if (node.kind !== 'image' || node.snapshot.type !== 'image-node') {
        outcomes.push({
          resultNodeId: input.resultNodeId,
          kind: 'rejected',
          reason: 'non-image-node',
        })
        continue
      }
      if (node.snapshot.pendingTaskId !== input.taskId) {
        outcomes.push({
          resultNodeId: input.resultNodeId,
          kind: 'rejected',
          reason: 'stale-task',
        })
        continue
      }

      const resolved = normalizeResolvedImageResult(input.result, node.snapshot)
      transaction.updateNode(input.resultNodeId, {
        width: resolved.width,
        sourceImageUrl: input.result.url,
        sourceMediaRef: input.result.mediaRef,
        sourceImageRatio: resolved.ratio,
        pendingTaskId: undefined,
        pendingResultIndex: undefined,
        pendingClientActionId: undefined,
        outputStatus: 'ready',
        outputImages: [],
      })
      outcomes.push({ resultNodeId: input.resultNodeId, kind: 'materialized' })
    }

    const summary = transaction.commit()
    const result = Object.freeze({
      didChange: summary.didChange,
      changedNodeIds: Object.freeze(
        summary.changedNodeIds.filter((nodeId): nodeId is PendingImageResultNodeId =>
          nodeId.startsWith('image-'),
        ),
      ),
      outcomes: Object.freeze(outcomes.map((outcome) => Object.freeze(outcome))),
    })
    if (result.didChange) this.#onMaterialized?.(store, result)
    return result
  }
}
