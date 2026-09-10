import type { DialogPassThroughOptions } from 'primevue/dialog'
import { getMotionPreference } from './motionPolicy'
import './dialog-motion.css'

export const dialogMotionTransitionName = 'ib-dialog-motion'

export function createDialogMotionPt(): Pick<DialogPassThroughOptions, 'transition'> {
  return {
    transition: {
      name: dialogMotionTransitionName,
      css: getMotionPreference() !== 'reduce',
    },
  }
}
