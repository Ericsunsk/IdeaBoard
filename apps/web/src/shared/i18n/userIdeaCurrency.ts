import { formatLocaleNumber } from './formatters'

export function formatIdeaAmount(credits: number): string {
  return `${formatLocaleNumber(credits)} ${credits === 1 ? 'Idea' : 'Ideas'}`
}
