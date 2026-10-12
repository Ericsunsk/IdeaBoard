<template>
  <canvas ref="canvasRef" class="composing-orb" width="171" height="171" aria-hidden="true" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getMotionPreference } from '../../../../../core/motion/motionPolicy'
import { COMPOSING_ORB_BASE_SPEED, drawComposingOrb } from './thinking-orbs/composingRenderer'

const ORB_SIZE = 171
const ORB_SPEED = 1.2
const STATIC_FRAME_SECONDS = 0.6
const canvasRef = ref<HTMLCanvasElement | null>(null)

let animationFrameId: number | null = null
let context: CanvasRenderingContext2D | null = null
let intersectionObserver: IntersectionObserver | null = null
let motionPreferenceQuery: MediaQueryList | null = null
let isIntersecting = true
let isReducedMotionPreferred = false

function drawFrame(timeSeconds: number) {
  if (!context) return
  const pixelRatio = Math.min(2, window.devicePixelRatio || 1)
  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
  context.clearRect(0, 0, ORB_SIZE, ORB_SIZE)
  drawComposingOrb(context, ORB_SIZE, timeSeconds)
}

function stopAnimation() {
  if (animationFrameId === null) return
  cancelAnimationFrame(animationFrameId)
  animationFrameId = null
}

function animationLoop(timestamp: number) {
  drawFrame((timestamp / 1000) * COMPOSING_ORB_BASE_SPEED * ORB_SPEED)
  animationFrameId = requestAnimationFrame(animationLoop)
}

function canAnimate() {
  return (
    !isReducedMotionPreferred &&
    isIntersecting &&
    document.visibilityState !== 'hidden' &&
    animationFrameId === null
  )
}

function reconcileAnimation() {
  if (!canAnimate()) {
    if (isReducedMotionPreferred || !isIntersecting || document.visibilityState === 'hidden') {
      stopAnimation()
    }
    return
  }
  animationFrameId = requestAnimationFrame(animationLoop)
}

function handleVisibilityChange() {
  reconcileAnimation()
}

function handleMotionPreferenceChange(event: MediaQueryListEvent) {
  isReducedMotionPreferred = event.matches
  if (isReducedMotionPreferred) drawFrame(STATIC_FRAME_SECONDS)
  reconcileAnimation()
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return

  const pixelRatio = Math.min(2, window.devicePixelRatio || 1)
  canvas.width = Math.round(ORB_SIZE * pixelRatio)
  canvas.height = Math.round(ORB_SIZE * pixelRatio)
  context = canvas.getContext('2d')
  if (!context) return

  isReducedMotionPreferred = getMotionPreference() === 'reduce'
  drawFrame(STATIC_FRAME_SECONDS)

  motionPreferenceQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  motionPreferenceQuery.addEventListener('change', handleMotionPreferenceChange)
  document.addEventListener('visibilitychange', handleVisibilityChange)

  intersectionObserver = new IntersectionObserver(([entry]) => {
    isIntersecting = entry?.isIntersecting ?? true
    reconcileAnimation()
  })
  intersectionObserver.observe(canvas)
  reconcileAnimation()
})

onBeforeUnmount(() => {
  stopAnimation()
  intersectionObserver?.disconnect()
  motionPreferenceQuery?.removeEventListener('change', handleMotionPreferenceChange)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.composing-orb {
  display: block;
  flex: 0 0 171px;
  width: 171px;
  height: 171px;
  opacity: 0.4;
}
</style>
