import { ref } from 'vue'
import { useEventListener } from '@vueuse/core'
import type { SearchResult } from '../core/models'

export function useSearchLightbox() {
  const visible = ref(false)
  const activeResult = ref<SearchResult | null>(null)

  function open(result: SearchResult) {
    activeResult.value = result
    visible.value = true
  }

  function close() {
    visible.value = false
    activeResult.value = null
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && visible.value) close()
  }

  // Listener lifetime follows the search session (activate/dispose), not the
  // component scope — `useEventListener` owns add/removeEventListener pairing.
  let stopKeydown: (() => void) | null = null

  function activate() {
    if (stopKeydown) return
    stopKeydown = useEventListener(window, 'keydown', handleKeydown)
  }

  function dispose() {
    stopKeydown?.()
    stopKeydown = null
    close()
  }

  return { visible, activeResult, open, close, activate, dispose }
}
