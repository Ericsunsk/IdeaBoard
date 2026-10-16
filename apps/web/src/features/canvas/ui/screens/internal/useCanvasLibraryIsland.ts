import { onScopeDispose, ref } from 'vue'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  createLibrarySaveModule,
  listAllWorkspaceLibraryFolders,
  requireWorkspaceId,
} from '../../../../library'
import { useAppToast } from '../../../../../core/composables/useAppToast'
import { getWorkspaceSelectionStore } from '../../../../workspace'

/** Owns Canvas-local Library visibility and its shared Library coordination facts. */
export function useCanvasLibraryIsland() {
  const isOpen = ref(false)
  const isMounted = ref(false)
  const isFocusWithin = ref(false)
  const folderFacts = createLibraryFolderFacts(() =>
    listAllWorkspaceLibraryFolders(
      requireWorkspaceId(getWorkspaceSelectionStore().getSelectedId()),
    ),
  )
  const mutationPublisher = createLibraryMutationPublisher(folderFacts)
  const saveModule = createLibrarySaveModule({
    workspaceId: () => getWorkspaceSelectionStore().getSelectedId(),
    mutationPublisher,
    notify: useAppToast(),
  })

  onScopeDispose(() => {
    saveModule.dispose()
    folderFacts.dispose()
  })

  return {
    view: { isOpen, isMounted, isFocusWithin, folderFacts, mutationPublisher, saveModule },
    commands: {
      toggle() {
        if (!isOpen.value) isMounted.value = true
        isOpen.value = !isOpen.value
        if (!isOpen.value) isFocusWithin.value = false
      },
      close() {
        isOpen.value = false
        isFocusWithin.value = false
      },
      setFocusWithin(isFocused: boolean) {
        isFocusWithin.value = isFocused
      },
    },
  }
}
