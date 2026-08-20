import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { SearchResult } from './searchResource'

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

  onMounted(() => window.addEventListener('keydown', handleKeydown))
  onBeforeUnmount(() => window.removeEventListener('keydown', handleKeydown))

  return { visible, activeResult, open, close }
}
