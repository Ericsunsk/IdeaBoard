import { computed, type Ref } from 'vue'
import type { Elements, Node } from '@vue-flow/core'

type UseFlowSelectionPreviewBarOptions = {
  nodes: Ref<Node[]>
  flowPromptText: Ref<string>
  flowPromptRatio: Ref<string>
  flowPromptQuality: Ref<string>
  clearFlowSelection: (elements?: Elements) => boolean
  getViewport: () => { x: number; y: number; zoom: number }
  setViewport: (vp: { x: number; y: number; zoom: number }) => void
  runPatternGenerate: (prompt: string, ratio: string, quality: string) => Promise<void>
}

export function useFlowSelectionPreviewBar(options: UseFlowSelectionPreviewBarOptions) {
  const selectedPreviewItems = computed(() => {
    return (options.nodes.value as Array<{ id: string; selected?: boolean; data?: { kind?: string; snapshotNode?: unknown } }>)
      .filter(node => node.selected && node.data?.kind === 'message' && node.data.snapshotNode && typeof node.data.snapshotNode === 'object')
      .map(node => {
        const snapshot = node.data!.snapshotNode as { id?: number; url?: string }
        return { id: Number(snapshot.id ?? 0), url: String(snapshot.url ?? '') }
      })
      .filter(item => item.id > 0 && item.url.length > 0)
  })

  function onClearSelected() {
    options.clearFlowSelection()
  }

  function onRemoveSelectedPreviewItem(id: number) {
    const nodeId = `message-${id}`
    const node = (options.nodes.value as Array<Node & { selected?: boolean }>).find(node => node.id === nodeId)
    if (!node?.selected) return
    // Deselect the node instead of deleting; Vue Flow owns selected state.
    options.clearFlowSelection([node])
  }

  function onFocusSelectedPreviewItem(id: number) {
    const nodeId = `message-${id}`
    const node = (options.nodes.value as Array<{ id: string; position: { x: number; y: number } }>).find(n => n.id === nodeId)
    if (!node) return
    const viewport = options.getViewport()
    options.setViewport({
      x: node.position.x - 300,
      y: node.position.y - 200,
      zoom: viewport.zoom,
    })
  }

  function onFlowGenerate() {
    const prompt = options.flowPromptText.value.trim()
    if (!prompt) return
    void options.runPatternGenerate(prompt, options.flowPromptRatio.value, options.flowPromptQuality.value)
  }

  return {
    selectedPreviewItems,
    onClearSelected,
    onRemoveSelectedPreviewItem,
    onFocusSelectedPreviewItem,
    onFlowGenerate,
  }
}
