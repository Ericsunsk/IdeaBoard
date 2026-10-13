import { type ClassValue, clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

/**
 * Theme-namespace guard for conflict resolution. tailwind-merge only dedupes
 * classes it recognises; every custom domain color token would otherwise
 * survive next to its opponent and leave the winner to stylesheet order.
 * Registering the namespaces keeps invalid-state borders deterministic when a
 * surface skin and semantic error class are composed.
 * With the namespace registered, `cn()` keeps the last one — the error state
 * always wins over the base chrome, on every surface.
 */
const isThemeColor = (value: string) =>
  /^(?:admin|board|page-chrome|overlay|auth|toast|brand-creative)-/.test(value)

const customTwMerge = extendTailwindMerge({
  extend: {
    theme: {
      radius: ['control', 'panel', 'overlay', 'pill'],
    },
    classGroups: {
      'border-color': [{ border: [isThemeColor] }],
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

export function cleanProps<T extends Record<string, unknown>>(props: T): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const key of Object.keys(props)) {
    if (props[key] !== undefined) {
      result[key] = props[key]
    }
  }
  return result
}
