import type { BoardStateMediaRef, BoardSnapshot } from '../document/nodes'

function resolveImageNodeMediaRefRole(base: BoardSnapshot | undefined, refId: string): string {
  return (
    base?.mediaRefs?.find(
      (ref) =>
        ref.refType === 'board_node' && ref.refId === refId && !ref.role?.startsWith('output:'),
    )?.role || 'source'
  )
}

export function rebuildBoardMediaRefs(
  snapshot: BoardSnapshot,
  base: BoardSnapshot | undefined = snapshot,
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

  function appendImageRef(
    image: { url: string; mediaAssetId?: number | undefined },
    refId: string,
    role: string,
  ) {
    if (!image.url.trim()) return
    appendRef(image.mediaAssetId, refId, role)
  }

  snapshot.imageNodes.forEach((node) => {
    const refId = `image-${node.id}`
    appendImageRef(
      { url: node.sourceImageUrl, mediaAssetId: node.sourceMediaAssetId },
      refId,
      resolveImageNodeMediaRefRole(base, refId),
    )
    // The retouch recipe source carries a string delivery identity, not a
    // numeric board ref. Its asset stays pinned through the node's own
    // source/output refs, so no separate numeric ref is emitted here.
    node.outputImages.forEach((output, index) => {
      appendImageRef(output, refId, `output:${index}`)
    })
  })
  snapshot.promptNodes.forEach((node) => {
    const refId = `prompt-${node.id}`
    node.inputImages?.forEach((input, index) => {
      appendImageRef(input, refId, `input:${index}`)
    })
  })
  snapshot.fusionNodes.forEach((node) => {
    const refId = `fusion-${node.id}`
    node.inputImages?.forEach((input, index) => {
      appendImageRef(input, refId, `input:${index}`)
    })
    node.outputImages.forEach((output, index) => {
      appendImageRef(output, refId, `output:${index}`)
    })
  })

  if (refs.length > 0) {
    snapshot.mediaRefs = refs
  } else {
    delete snapshot.mediaRefs
  }
}
