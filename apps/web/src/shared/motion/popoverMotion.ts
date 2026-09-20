import { getMotionPreference } from './motionPolicy'
import './popover-motion.css'

export const popoverMotionTransitionName = 'ib-popover-motion'

export type PopoverMotionTransition = {
  name: string
  css: boolean
}

export function createPopoverMotionTransition(): PopoverMotionTransition {
  return {
    name: popoverMotionTransitionName,
    css: getMotionPreference() !== 'reduce',
  }
}
