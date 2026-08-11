import { onMounted, onScopeDispose } from 'vue'
import { useCanvasBoardCatalog } from '../canvas-board/catalog/canvasBoardCatalog'
import { createBoardPreviewRefreshQueue } from '../canvas-board/preview'
import {
  createBoardCatalogController,
  formatBoardCatalogDate,
} from './controller/boardCatalogController'
import type { BoardsCatalog, BoardsCatalogNavigation } from './boardCatalogContract'
import { useBoardFeedback } from './feedback/useBoardFeedback'

export type {
  BoardCatalogEntry,
  BoardCatalogSortMode,
  BoardsCatalog,
  BoardsCatalogNavigation,
} from './boardCatalogContract'
export { formatBoardCatalogDate }

export function createBoardsCatalog(options: {
  navigation: BoardsCatalogNavigation
}): BoardsCatalog {
  return createBoardCatalogController({
    catalog: useCanvasBoardCatalog(),
    preview: createBoardPreviewRefreshQueue(),
    navigation: options.navigation,
    feedback: useBoardFeedback(),
    warn: console.warn,
  })
}

export function useBoardsCatalog(options: { navigation: BoardsCatalogNavigation }): BoardsCatalog {
  const catalog = createBoardsCatalog(options)
  onMounted(() => {
    void catalog.commands.initialize()
  })
  onScopeDispose(catalog.dispose)
  return catalog
}
