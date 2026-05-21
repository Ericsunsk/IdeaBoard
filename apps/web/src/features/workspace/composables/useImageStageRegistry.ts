export type ImageStageRegistry = {
  setImageStageRef(nodeId: string, element: Element | null): void
  clearImageStageRef(nodeId: string, element?: Element | null): void
  getImageStageElement(nodeId: string): HTMLElement | null
}

export function useImageStageRegistry(): ImageStageRegistry {
  const stageMap = new Map<string, HTMLElement>()

  function setImageStageRef(nodeId: string, element: Element | null): void {
    if (!nodeId) return
    if (!element || !(element instanceof HTMLElement)) {
      stageMap.delete(nodeId)
      return
    }
    stageMap.set(nodeId, element)
  }

  function clearImageStageRef(nodeId: string, element?: Element | null): void {
    if (!nodeId) return
    if (element instanceof HTMLElement && stageMap.get(nodeId) !== element) return
    stageMap.delete(nodeId)
  }

  function getImageStageElement(nodeId: string): HTMLElement | null {
    if (!nodeId) return null
    return stageMap.get(nodeId) ?? null
  }

  return {
    setImageStageRef,
    clearImageStageRef,
    getImageStageElement,
  }
}
