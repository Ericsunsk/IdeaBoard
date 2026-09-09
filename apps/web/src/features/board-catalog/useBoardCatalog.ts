import { onMounted, onScopeDispose } from 'vue'
import { useCanvasBoardCatalog } from '../canvas-board/catalog/catalog'
import { createBoardPreviewRefreshQueue } from '../canvas-board/preview'
import {
  createController,
  formatCatalogDate,
} from './controller/controller'
import type { BoardCatalog, BoardCatalogNavigation } from './contract'
import { useBoardFeedback } from './feedback/useFeedback'

export type {
  BoardCatalogEntry,
  BoardCatalogSortMode,
  BoardCatalog,
  BoardCatalogNavigation,
} from './contract'
export { formatCatalogDate }

export function createBoardCatalog(options: {
  navigation: BoardCatalogNavigation
}): BoardCatalog {
  return createController({
    catalog: useCanvasBoardCatalog(),
    preview: createBoardPreviewRefreshQueue(),
    navigation: options.navigation,
    feedback: useBoardFeedback(),
    warn: console.warn,
  })
}

export function useBoardCatalog(options: { navigation: BoardCatalogNavigation }): BoardCatalog {
  const catalog = createBoardCatalog(options)
  onMounted(() => {
    void catalog.commands.initialize()
  })
  onScopeDispose(catalog.dispose)
  return catalog
}
