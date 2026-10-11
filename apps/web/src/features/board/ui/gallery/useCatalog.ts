import { onMounted, onScopeDispose } from 'vue'
import { useBoardRepository } from '../../session/catalog/repository'
import { createBoardPreviewRefreshQueue } from '../../session/preview'
import { createGalleryController } from './controller'
import { formatCatalogDate } from './format'
import type { Catalog, CatalogNavigation } from './contract'
import { useBoardFeedback } from './useBoardFeedback'

import { useTheme } from '../../../../theme/composables/useTheme'

export type { CatalogSortMode, Catalog, CatalogNavigation } from './contract'
export { formatCatalogDate }

export function createCatalog(options: { navigation: CatalogNavigation }): Catalog {
  return createGalleryController({
    catalog: useBoardRepository(),
    preview: createBoardPreviewRefreshQueue(),
    navigation: options.navigation,
    feedback: useBoardFeedback(),
    theme: useTheme(),
    warn: console.warn,
  })
}

export function useCatalog(options: { navigation: CatalogNavigation }): Catalog {
  const catalog = createCatalog(options)
  onMounted(() => {
    void catalog.commands.initialize()
  })
  onScopeDispose(() => {
    catalog.dispose()
  })
  return catalog
}
