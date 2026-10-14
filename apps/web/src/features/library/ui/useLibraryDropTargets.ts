import { onBeforeUnmount, ref, type Directive } from 'vue'
import type { LibraryDragPayload, LibraryDropTargets } from '../core/drag'

type LibraryDropOptions = {
  targets: () => LibraryDropTargets
  isOpen: () => boolean
  onFolderHover: (key: string | null) => void
  onDrop: (payload: LibraryDragPayload, folderKey?: string) => void
}

export function useLibraryDropTargets(options: LibraryDropOptions) {
  const registrations = new Map<
    HTMLElement,
    {
      targets: LibraryDropTargets
      folderKey: string | undefined
      unregister: () => void
    }
  >()
  const isPanelHovered = ref(false)

  function unregister(element: HTMLElement) {
    registrations.get(element)?.unregister()
    registrations.delete(element)
  }

  function register(element: HTMLElement, folderKey: string | undefined) {
    const targets = options.targets()
    const current = registrations.get(element)
    if (current?.targets === targets && current?.folderKey === folderKey) return
    unregister(element)
    const dispose = targets.register({
      element,
      canReceive: options.isOpen,
      onHoverChange(isHovered) {
        if (folderKey === undefined) isPanelHovered.value = isHovered
        else options.onFolderHover(isHovered ? folderKey : null)
      },
      onDrop(payload) {
        options.onDrop(payload, folderKey)
      },
    })
    registrations.set(element, { targets, folderKey, unregister: dispose })
  }

  const directive: Directive<HTMLElement, string | undefined> = {
    mounted: (element, binding) => register(element, binding.value),
    updated: (element, binding) => register(element, binding.value),
    beforeUnmount: unregister,
  }

  onBeforeUnmount(() => {
    for (const element of registrations.keys()) unregister(element)
  })

  return { directive, isPanelHovered }
}
