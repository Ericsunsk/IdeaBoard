import { type ClassValue, clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

/**
 * Theme-namespace guard for conflict resolution. tailwind-merge only dedupes
 * classes it recognises; every custom `--color-ib-*` / `--color-admin-*`
 * token would otherwise survive next to its opponent and leave the winner to
 * stylesheet order. That is how an invalid input kept its default grey border:
 * `border-ib-border` and `border-ib-danger` tied at (0,1,0) and order decided.
 * With the namespace registered, `cn()` keeps the last one — the error state
 * always wins over the base chrome, on every surface.
 */
const isThemeColor = (value: string) => /^(ib|admin)-/.test(value)

const customTwMerge = extendTailwindMerge({
  extend: {
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
