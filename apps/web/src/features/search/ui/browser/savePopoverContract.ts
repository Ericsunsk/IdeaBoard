import type { SearchResult } from '../../core/models'

export type SearchSavePopoverRequest = {
  result: SearchResult
  isSaved: boolean
  isAuthenticated: boolean
}

export type SearchSavePopoverController = {
  toggle: (event: Event, request: SearchSavePopoverRequest) => void
  hide: () => void
}

export function isSavePopoverController(value: unknown): value is SearchSavePopoverController {
  return (
    typeof value === 'object' &&
    value !== null &&
    'toggle' in value &&
    typeof value.toggle === 'function' &&
    'hide' in value &&
    typeof value.hide === 'function'
  )
}
