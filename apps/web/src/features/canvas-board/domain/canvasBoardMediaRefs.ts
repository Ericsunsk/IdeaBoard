import type { BoardStateMediaRef, CanvasBoardSnapshot } from './canvasNodes'

function resolveImageNodeMediaRefRole(
  base: CanvasBoardSnapshot | undefined,
  refId: string,
): string {
  return (
    base?.mediaRefs?.find(
      (ref) =>
        ref.refType === 'board_node' &&
        ref.refId === refId &&
        !ref.role?.startsWith('output:'),
    )?.role || 'source'
  )
}

export function rebuildCanvasBoardMediaRefs(
  snapshot: CanvasBoardSnapshot,
  base: CanvasBoardSnapshot | undefined = snapshot,
): void {
  const refs: BoardStateMediaRef[] = (snapshot.mediaRefs ?? base?.mediaRefs ?? [])
    .filter((ref) => ref.refType === 'board')
    .map((ref) => ({ ...ref }))
  const seen = new Set(
    refs.map((ref) => `${ref.mediaAssetId}:${ref.refType}:${ref.refId}:${ref.role ?? ''}`),
  )

  function appendRef(mediaAssetIdValue: unknown, refId: string, role: string) {
    const mediaAssetId = Number(mediaAssetIdValue)
    if (!Number.isInteger(mediaAssetId) || mediaAssetId <= 0) return
    const ref: BoardStateMediaRef = {
      mediaAssetId,
      refType: 'board_node',
      refId,
      role,
    }
    const key = `${ref.mediaAssetId}:${ref.refType}:${ref.refId}:${ref.role}`
    if (seen.has(key)) return
    seen.add(key)
    refs.push(ref)
  }

  snapshot.imageNodes.forEach((node) => {
    const refId = `image-${node.id}`
    appendRef(node.sourceMediaAssetId, refId, resolveImageNodeMediaRefRole(base, refId))
    node.outputImages.forEach((output, index) => {
      appendRef(output.mediaAssetId, refId, `output:${index}`)
    })
  })
  snapshot.promptNodes.forEach((node) => {
    const refId = `prompt-${node.id}`
    node.inputImages?.forEach((input, index) => {
      appendRef(input.mediaAssetId, refId, `input:${index}`)
    })
  })
  snapshot.fusionNodes.forEach((node) => {
    const refId = `fusion-${node.id}`
    node.inputImages?.forEach((input, index) => {
      appendRef(input.mediaAssetId, refId, `input:${index}`)
    })
    node.outputImages.forEach((output, index) => {
      appendRef(output.mediaAssetId, refId, `output:${index}`)
    })
  })

  if (refs.length > 0) {
    snapshot.mediaRefs = refs
  } else {
    delete snapshot.mediaRefs
  }
}
