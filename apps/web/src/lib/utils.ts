import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
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
