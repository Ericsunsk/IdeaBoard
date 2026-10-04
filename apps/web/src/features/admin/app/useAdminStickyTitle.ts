import { onMounted, onUnmounted, ref } from 'vue'

export function useAdminStickyTitle() {
  const showStickyTitle = ref(false)
  const dynamicTitle = ref('')
  let scrollTicking = false

  function updateStickyTitle() {
    if (typeof window === 'undefined') return
    if (window.scrollY <= 10) {
      showStickyTitle.value = false
      return
    }
    showStickyTitle.value = true
    if (typeof document === 'undefined') return
    const heading = document.querySelector('main h1')
    const text = heading?.textContent?.replace(/\s+/g, ' ').trim()
    dynamicTitle.value = text || ''
  }

  function handleScroll() {
    if (scrollTicking) return
    scrollTicking = true
    requestAnimationFrame(() => {
      updateStickyTitle()
      scrollTicking = false
    })
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('scroll', handleScroll, { passive: true })
      window.addEventListener('resize', handleScroll, { passive: true })
    }
    updateStickyTitle()
  })

  onUnmounted(() => {
    if (typeof window !== 'undefined') {
      window.removeEventListener('scroll', handleScroll)
      window.removeEventListener('resize', handleScroll)
    }
  })

  function reset() {
    showStickyTitle.value = false
    dynamicTitle.value = ''
  }

  return { showStickyTitle, dynamicTitle, reset }
}
