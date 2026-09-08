import { ref } from 'vue'
import type { SearchResult } from '../domain/models'

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

  let active = false

  function activate() {
    if (active) return
    active = true
    window.addEventListener('keydown', handleKeydown)
  }

  function dispose() {
    if (!active) return
    active = false
    window.removeEventListener('keydown', handleKeydown)
    close()
  }

  return { visible, activeResult, open, close, activate, dispose }
}
