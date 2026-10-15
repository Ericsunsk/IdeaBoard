import { effectScope, onMounted, onScopeDispose } from 'vue'
import { useBoardRepository } from '../../session/catalog/repository'
import { useBoardCatalogScenes } from '../../session/sync/boardQueries'
import { createGalleryController } from './controller'
import { formatCatalogDate } from './format'
import type { Catalog, CatalogNavigation } from './contract'
import { useBoardFeedback } from './useBoardFeedback'

export type { CatalogSortMode, Catalog, CatalogNavigation } from './contract'
export { formatCatalogDate }

export function createCatalog(options: { navigation: CatalogNavigation }): Catalog {
  const scope = effectScope(true)
  const catalog = useBoardRepository()
  const controller = scope.run(() =>
    createGalleryController({
      catalog,
      scenes: useBoardCatalogScenes(catalog.viewModel.boards),
      navigation: options.navigation,
      feedback: useBoardFeedback(),
      warn: console.warn,
    }),
  )!
  return {
    ...controller,
    dispose() {
      controller.dispose()
      scope.stop()
    },
  }
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
