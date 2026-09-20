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
            'ib-micro',
            'ib-caption',
            'ib-body',
          ],
        },
      ],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return customTwMerge(clsx(inputs))
}

export function cleanProps<T extends Record<string, unknown>>(props: T): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const key of Object.keys(props)) {
    if (props[key] !== undefined) {
      result[key] = props[key]
    }
  }
  return result
}
