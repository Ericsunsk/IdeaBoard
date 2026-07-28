import type { PopoverPassThroughOptions } from 'primevue/popover'
import { getMotionPreference } from './motionPolicy'
import './popoverMotion.css'

export const popoverMotionTransitionName = 'ib-popover-motion'

export function createPopoverMotionPt() {
  return {
    transition: {
      name: popoverMotionTransitionName,
      css: getMotionPreference() !== 'reduce',
    },
  } satisfies Pick<PopoverPassThroughOptions, 'transition'>
}
