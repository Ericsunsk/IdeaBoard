import { onMounted, onScopeDispose } from 'vue'
import { useBoardRepository } from '../../application/catalog/repository'
import { createBoardPreviewRefreshQueue } from '../../infrastructure/preview'
import { createGalleryController } from './galleryController'
import { formatCatalogDate } from './format'
import type { Catalog, CatalogNavigation } from './contract'
import { useBoardFeedback } from './useBoardFeedback'

export type {
  CatalogEntry,
  CatalogSortMode,
  Catalog,
  CatalogNavigation,
} from './contract'
export { formatCatalogDate }

export function createCatalog(options: {
  navigation: CatalogNavigation
}): Catalog {
  return createGalleryController({
    catalog: useBoardRepository(),
    preview: createBoardPreviewRefreshQueue(),
    navigation: options.navigation,
    feedback: useBoardFeedback(),
    warn: console.warn,
  })
}

export function useCatalog(options: { navigation: CatalogNavigation }): Catalog {
  const catalog = createCatalog(options)
  onMounted(() => {
    void catalog.commands.initialize()
  })
  onScopeDispose(catalog.dispose)
  return catalog
}
