import type { PopoverPassThroughOptions } from 'primevue/popover'
import { getMotionPreference } from './motionPolicy'
import './popover-motion.css'

export const popoverMotionTransitionName = 'ib-popover-motion'

export function createPopoverMotionTransition() {
  return {
    name: popoverMotionTransitionName,
    css: getMotionPreference() !== 'reduce',
  }
}

export function createPopoverMotionPt() {
  return {
    transition: createPopoverMotionTransition(),
  } satisfies Pick<PopoverPassThroughOptions, 'transition'>
}
