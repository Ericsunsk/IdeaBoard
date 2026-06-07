import { onBeforeUnmount, ref } from 'vue'

export function useTransientScrolling(delay = 700) {
  const isScrolling = ref(false)
  let scrollTimer: number | undefined

  function handleScroll() {
    isScrolling.value = true
    if (scrollTimer) window.clearTimeout(scrollTimer)
    scrollTimer = window.setTimeout(() => {
      isScrolling.value = false
      scrollTimer = undefined
    }, delay)
  }

  onBeforeUnmount(() => {
    if (scrollTimer) {
      window.clearTimeout(scrollTimer)
      scrollTimer = undefined
    }
  })

  return { isScrolling, handleScroll }
}
