import { type ClassValue, clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

const customTwMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [
        {
          text: [
            'admin-micro',
            'admin-caption',
            'admin-meta',
            'admin-body',
            'admin-content',
            'admin-title-sm',
            'admin-title',
            'admin-heading',
            'admin-icon',
            'admin-summary-value',
            'admin-modal-title',
            'admin-modal-title-mobile',
          ],
        },
      ],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return customTwMerge(clsx(inputs))
}
