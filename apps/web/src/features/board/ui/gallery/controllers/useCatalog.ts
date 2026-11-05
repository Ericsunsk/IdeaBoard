import { effectScope, onMounted, onScopeDispose } from 'vue'
import { useBoardRepository } from '../../../session/catalog/repository'
import { useBoardCatalogScenes } from '../../../session/sync/boardQueries'
import { createGalleryController } from './galleryController'
import type { Catalog, CatalogNavigation } from '../styling/contract'
import { useBoardFeedback } from './useBoardFeedback'

export function useCatalog(options: { navigation: CatalogNavigation }): Catalog {
  const scope = effectScope(true)
  const repository = useBoardRepository()
  const controller = scope.run(() =>
    createGalleryController({
      catalog: repository,
      scenes: useBoardCatalogScenes(repository.viewModel.boards),
      navigation: options.navigation,
      feedback: useBoardFeedback(),
      warn: console.warn,
    }),
  )!
  const catalog: Catalog = {
    ...controller,
    dispose() {
      controller.dispose()
      scope.stop()
    },
  }
  onMounted(() => {
    void catalog.commands.initialize()
  })
  onScopeDispose(() => {
    catalog.dispose()
  })
  return catalog
}
