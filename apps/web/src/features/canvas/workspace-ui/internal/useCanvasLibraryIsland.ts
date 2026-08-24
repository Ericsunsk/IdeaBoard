import { onScopeDispose, ref } from 'vue'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
} from '../../../library/libraryFolderFacts'

/** Owns Canvas-local Library visibility and its shared Library coordination facts. */
export function useCanvasLibraryIsland() {
  const open = ref(false)
  const mounted = ref(false)
  const focusWithin = ref(false)
  const folderFacts = createLibraryFolderFacts()
  const mutationPublisher = createLibraryMutationPublisher(folderFacts)

  onScopeDispose(folderFacts.dispose)

  return {
    view: { open, mounted, focusWithin, folderFacts, mutationPublisher },
    commands: {
      toggle() {
        if (!open.value) mounted.value = true
        open.value = !open.value
        if (!open.value) focusWithin.value = false
      },
      close() {
        open.value = false
        focusWithin.value = false
      },
      setFocusWithin(focused: boolean) {
        focusWithin.value = focused
      },
    },
  }
}
