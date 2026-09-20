<template>
  <div
    ref="pageScrollRef"
    class="app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="sticky top-0 z-40 flex items-center justify-between pointer-events-none pb-2">
      <StudioName class="pointer-events-auto" />
      <WorkspaceNavigation class="pointer-events-auto" />
      <UserAccountMenu
        class="pointer-events-auto"
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      >
        <template #before-balance>
          <UserLocaleSelect />
        </template>
      </UserAccountMenu>
    </header>

    <!-- 🌟 主视窗容器：当左侧素材库浮岛展开时，右侧紧贴浮岛右侧保留标准 24px 呼吸间距；全屏展开时 1:1 物理被顶出右侧屏幕 -->
    <main
      class="relative z-[1] mt-12 will-change-[transform,margin,padding] transition-[transform,margin,padding] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] [transform:translateZ(0)]"
      :class="[
        libraryIslandOpen && !libraryIslandExpanded
          ? 'ml-0 md:ml-[350px] mr-auto max-w-[1600px] pl-0 md:pl-6 lg:pl-8'
          : 'mx-auto max-w-[1600px] pl-0',
        libraryIslandOpen && libraryIslandExpanded ? 'translate-x-[100vw] pointer-events-none' : '',
      ]"
    >
      <!-- 🌟 核心吸顶搜索控制中心 (保持原始呼吸间距，向下滚动时悬浮吸顶) -->
      <div class="sticky top-16 z-30 flex flex-col items-center mb-6 pointer-events-none">
        <div class="flex w-full max-w-[720px] items-center gap-2 pointer-events-auto">
          <form
            class="ib-page-chrome-surface min-w-0 flex-1 rounded-full p-1.5"
            @submit.prevent="submitQuery"
          >
            <div class="flex items-center gap-2">
              <Search
                v-if="references.length === 0"
                class="ml-2 size-3.5 shrink-0 text-board-control-icon"
                aria-hidden="true"
              />

              <div v-else class="flex items-center gap-1.5 shrink-0">
                <div
                  v-for="reference in references"
                  :key="reference.key"
                  class="group/thumb relative shrink-0"
                >
                  <button
                    type="button"
                    class="grid size-8 overflow-hidden rounded-full bg-board-card-bg cursor-pointer transition-[transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-board-primary"
                    :title="
                      t('search.tooltips.removeReferenceWithCount', { count: referenceSlotCount })
                    "
                    :aria-label="t('search.tooltips.removeReference')"
                    @click="removeReference(reference.key)"
                  >
                    <img
                      :src="reference.previewUrl"
                      :alt="t('search.reference.imageAlt')"
                      class="size-full object-cover select-none pointer-events-none"
                    />
                  </button>

                  <div
                    class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-4 z-50 w-14 opacity-0 scale-95 transition-[opacity,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover/thumb:opacity-100 group-hover/thumb:scale-100 origin-bottom"
                  >
                    <div
                      class="size-14 shrink-0 overflow-hidden rounded-md bg-board-card-bg shadow-md"
                    >
                      <img
                        :src="reference.previewUrl"
                        :alt="t('search.reference.previewAlt')"
                        class="size-full block object-cover select-none pointer-events-none"
                      />
                    </div>
                  </div>
                </div>

                <button
                  v-if="canAddReference"
                  type="button"
                  class="search-reference-control"
                  :title="
                    t('search.tooltips.addReferencesWithCount', { count: referenceSlotCount })
                  "
                  :aria-label="
                    t('search.tooltips.addReferencesWithCount', { count: referenceSlotCount })
                  "
                  @click="openFilePicker"
                >
                  <Plus class="size-3.5" aria-hidden="true" />
                </button>
              </div>

              <input
                v-model="draftQuery"
                type="text"
                class="min-w-0 flex-1 bg-transparent px-1 text-[13.5px] text-board-control-input-text outline-none"
                :placeholder="
                  references.length > 0
                    ? t('search.placeholders.queryWithReferences')
                    : t('search.placeholders.queryDefault')
                "
                :aria-label="t('search.inputLabel')"
                @input="queryError = ''"
              />

              <div class="flex items-center gap-1.5 shrink-0">
                <button
                  v-if="references.length > 0 || draftQuery"
                  type="button"
                  class="search-reference-control"
                  :title="t('search.tooltips.clearSearch')"
                  :aria-label="t('search.tooltips.clearSearch')"
                  @click="handleClearSearch"
                >
                  <Xmark class="size-3.5" aria-hidden="true" />
                </button>
                <button
                  v-if="references.length === 0"
                  type="button"
                  class="h-8 px-3 rounded-full border border-[var(--color-ib-page-chrome-border)] text-[var(--color-ib-page-chrome-color)] hover:text-[var(--color-ib-page-chrome-color-hover)] hover:border-[var(--color-ib-page-chrome-border-hover)] hover:bg-[var(--color-ib-page-chrome-bg-hover)] flex items-center gap-1.5 text-xs font-medium transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                  :aria-label="t('search.tooltips.addReferences')"
                  :title="t('search.tooltips.addReferences')"
                  :disabled="!canAddReference"
                  @click="openFilePicker"
                >
                  <Camera class="size-3.5 text-board-primary" aria-hidden="true" />
                  <span class="hidden sm:inline">{{ t('search.fallbackImageTitle') }}</span>
                </button>
                <Button
                  type="submit"
                  :loading="visionState === 'extracting'"
                  :disabled="visionState === 'extracting'"
                  :title="searchButtonTitle"
                  class="!h-8 !min-w-[68px] !rounded-full !border-0 !bg-board-primary !px-4 !text-xs !font-bold !text-ib-text-inverse !shadow-none hover:!bg-board-primary-hover cursor-pointer disabled:!cursor-wait disabled:!opacity-60"
                >
                  {{ t('search.actions.search') }}
                </Button>
              </div>
            </div>
          </form>

          <button
            type="button"
            class="ib-page-chrome-surface group grid size-11 shrink-0 place-items-center rounded-full transition-[transform,border-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105 active:scale-95 hover:border-[var(--color-ib-page-chrome-border-hover)] focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_var(--color-ib-page-chrome-focus-ring)] cursor-pointer"
            :class="[isTriggerHoveredDuringDrag ? '!scale-110 !border-board-primary' : '']"
            :aria-label="t('search.tooltips.toggleLibrary')"
            :title="t('search.tooltips.toggleLibrary')"
            :aria-expanded="libraryIslandOpen"
            @pointerenter="preloadLibraryIsland"
            @focus="preloadLibraryIsland"
            @click="toggleLibraryIslandForUser"
            @dragover.prevent="handleLibraryTriggerDragOver"
            @dragleave="handleLibraryTriggerDragLeave"
            @drop.prevent="handleLibraryTriggerDrop"
          >
            <BookmarkSolid
              v-if="libraryIslandOpen"
              class="size-4 text-board-primary rotate-6 scale-110 transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              aria-hidden="true"
            />
            <Bookmark
              v-else
              class="size-4 text-[var(--color-ib-page-chrome-color)] transition-[transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover:text-[var(--color-ib-page-chrome-color-hover)] group-hover:rotate-6 group-hover:scale-105"
              aria-hidden="true"
            />
          </button>
        </div>

        <input
          ref="fileInput"
          type="file"
          accept="image/*"
          multiple
          class="hidden"
          :aria-label="t('search.tooltips.addReferences')"
          @change="handleFileInput"
        />

        <p
          v-if="queryError"
          class="mt-2 mb-0 text-sm font-semibold text-board-danger pointer-events-auto"
        >
          {{ queryError }}
        </p>
      </div>

      <div class="mt-6 flex items-center justify-between gap-3">
        <div>
          <h2 class="m-0 text-[22px] font-semibold">
            {{ searchResultsHeading }}
          </h2>
          <div
            v-if="hasError && results.length > 0"
            class="mt-1 flex items-center gap-2 text-sm text-board-danger"
          >
            <span>{{ t('search.errors.unableToLoadResults') }}</span>
            <button type="button" class="font-semibold text-board-primary" @click="reload">
              {{ t('search.actions.retry') }}
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="loading && results.length === 0"
        class="mt-4 transition-[column-gap] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)]"
        :class="[
          libraryIslandOpen && !libraryIslandExpanded
            ? 'columns-1 sm:columns-2 md:columns-3 lg:columns-4 xl:columns-4 gap-3.5'
            : 'columns-1 sm:columns-2 md:columns-3 lg:columns-4 xl:columns-5 gap-4',
        ]"
      >
        <div
          v-for="h in [260, 340, 200, 380, 280, 320, 240, 360, 300, 220]"
          :key="h"
          class="break-inside-avoid overflow-hidden rounded-2xl bg-board-loading-bg/80 animate-pulse"
          :class="libraryIslandOpen && !libraryIslandExpanded ? 'mb-3.5' : 'mb-4'"
          :style="{ height: `${h}px` }"
        ></div>
      </div>

      <SearchEmptyState
        v-else-if="results.length === 0"
        :has-error="hasError"
        :results-count="results.length"
        :has-active-search="hasActiveSearch"
        :can-add-reference="canAddReference"
        @reload="reload"
        @clear-search="handleClearSearch"
        @open-file-picker="openFilePicker"
        @start-creating="handleStartCreating"
      />

      <!-- Virtualized responsive Masonry result viewport -->
      <SearchResultViewport
        v-else
        :items="results"
        :has-more="hasMore"
        :loading="loading || loadingMore"
        :compact="libraryIslandOpen && !libraryIslandExpanded"
        :scroll-root="pageScrollRef"
        :active-anchor="savePopoverAnchor"
        class="mt-4"
        @active-anchor-hidden="libraryCommands.handleAnchorHidden"
        @load-next-page="loadNextPage"
        @scroll-position="recordScroll"
      >
        <template #default="{ item: result, index, style }">
          <SearchResultCard
            :result="result"
            :index="index"
            :card-style="style"
            :is-selected="isResultSelected(result)"
            :is-used-as-reference="
              references.some((ref) => ref.key === searchReferenceKey(result.provider, result.id))
            "
            :is-saved="savedKeys.has(libraryAssetKey(result.provider, result.id))"
            :has-image-error="failedImages.has(result.id)"
            :image-retry-count="imageRetryCount(result.id)"
            @card-drag-start="handleCardDragStart($event, result)"
            @card-drag-end="handleCardDragEnd"
            @open-lightbox="openLightbox(result)"
            @mark-image-error="markImageError(result.id)"
            @toggle-select="toggleBatchSelect(result)"
            @use-as-reference="handleSearchResultReference(result)"
            @toggle-save="toggleSavePopover($event, result)"
            @retry-image="retryImage(result.id)"
          />
        </template>
      </SearchResultViewport>

      <div v-if="loadingMore" class="flex justify-center py-5">
        <Spinner class="!h-7 !w-7 text-board-primary" :label="t('search.loadingMore')" />
      </div>
      <p
        v-else-if="results.length > 0 && !hasMore"
        class="py-4 text-center text-xs text-board-page-text-muted"
      >
        {{ t('search.allLoaded') }}
      </p>
    </main>

    <!-- 🌟 左侧悬浮圆角素材岛组件 -->
    <LibraryFloatingIsland
      v-if="libraryIslandMounted"
      v-model:expanded="libraryIslandExpanded"
      presentation="search"
      :open="libraryIslandOpen"
      :folder-facts="libraryFolderFacts"
      :mutation-publisher="libraryMutationPublisher"
      :reference-keys="references.map((reference) => reference.key)"
      @close="closeLibraryIsland"
      @reference-selected="handleLibraryReference"
    />

    <SearchSavePopover
      :ref="libraryCommands.registerSavePopover"
      @auth-required="libraryCommands.requestSaveAuthentication"
      @anchor-changed="libraryCommands.setSavePopoverAnchor"
    />

    <!-- 🌟 Demo 同款零遮挡全景高清 Lightbox 弹窗 -->
    <SearchLightboxModal
      :visible="lightboxVisible"
      :active-result="activeResult"
      @close="closeLightbox"
    />

    <!-- 🌟 多选批量悬浮操作岛 (SearchBatchActionDock) -->
    <SearchBatchActionDock
      :selected-count="selectedBatchCount"
      @vision-search="applySelectedAsVisionReferences"
      @save-all="saveAllSelectedToLibrary"
      @clear="clearSelection"
    />
  </div>
</template>

<script setup lang="ts">
import '../search.css'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Bookmark, BookmarkSolid, Camera, Plus, Search, Xmark } from '@iconoir/vue'
import StudioName from '../../../../shared/components/StudioName.vue'
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import UserLocaleSelect from '../../../../shared/components/UserLocaleSelect.vue'
import { HOME_PATH, USER_BOARDS_PATH } from '../../../../shared/constants/routePaths'
import { useUserStore } from '../../../auth/session/userSession'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { WorkspaceNavigation, useWorkspaceAccountControls } from '../../../workspace'
import SearchSavePopover from '../components/SearchSavePopover.vue'
import SearchBatchActionDock from '../components/SearchBatchActionDock.vue'
import SearchResultViewport from '../components/SearchResultViewport.vue'
import SearchResultCard from '../components/SearchResultCard.vue'
import SearchEmptyState from '../components/SearchEmptyState.vue'
import SearchLightboxModal from '../components/SearchLightboxModal.vue'
import { libraryAssetKey } from '../../../library'
import { searchReferenceKey } from '../../infrastructure/libraryBridge'
import { useAuthOverlay } from '../../../auth/overlay'
import {
  SearchLibraryFloatingIsland as LibraryFloatingIsland,
  useSearchScreenRuntime,
} from '../useSearchScreenRuntime'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const userStore = useUserStore()
const authOverlay = useAuthOverlay()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const accountControls = useWorkspaceAccountControls({
  navigation: { navigate: (path) => router.push(path), logout: () => logoutAndRedirect(HOME_PATH) },
})
const pageScrollRef = ref<HTMLElement | null>(null)
const userPublicID = computed(() => userStore.user?.public_id)

async function handleStartCreating() {
  const user = await userStore.ensureSession({ fresh: true }).catch(() => null)
  if (user) {
    await router.push(USER_BOARDS_PATH)
    return
  }
  authOverlay.commands.open('login', USER_BOARDS_PATH)
}
const searchScreen = useSearchScreenRuntime({
  userPublicID,
  scrollContainer: pageScrollRef,
  isAuthenticated: () => Boolean(userStore.user),
})
const { search, results: searchResults, library, selection, preview } = searchScreen.view
const {
  search: searchCommands,
  results: resultsCommands,
  selection: selectionCommands,
  library: libraryCommands,
} = searchScreen.commands
const { draftQuery, queryError, heading: searchResultsHeading } = search
const {
  references,
  referenceSlotCount,
  canAddReference,
  fileInput,
  visionState,
  buttonTitle: searchButtonTitle,
} = search
const hasActiveSearch = computed(() => {
  const queryFromRoute = String(route.query.q || '').trim()
  return Boolean(draftQuery.value.trim() || queryFromRoute || references.value.length > 0)
})
const {
  items: results,
  hasMore,
  loading,
  loadingMore,
  hasError,
  failedImages,
  savedKeys,
} = searchResults
const {
  open: libraryIslandOpen,
  expanded: libraryIslandExpanded,
  folderFacts: libraryFolderFacts,
  mutationPublisher: libraryMutationPublisher,
  mounted: libraryIslandMounted,
  isDropTarget: isTriggerHoveredDuringDrag,
} = library
const { count: selectedBatchCount } = selection
const { visible: lightboxVisible, activeResult } = preview
const { savePopoverAnchor } = searchScreen
const {
  clear: handleClearSearch,
  submit: submitQuery,
  openFilePicker,
  handleFileInput,
  removeReference,
  useResultAsReference: handleSearchResultReference,
  useLibraryReference: handleLibraryReference,
} = searchCommands
const {
  reload,
  loadNextPage,
  recordScroll,
  markImageError,
  retryImage,
  imageRetryCount,
  openPreview: openLightbox,
  closePreview: closeLightbox,
} = resultsCommands
const {
  isSelected: isResultSelected,
  toggle: toggleBatchSelect,
  clear: clearSelection,
  useAsVisionReferences: applySelectedAsVisionReferences,
  saveAllToLibrary: saveAllSelectedToLibrary,
} = selectionCommands
const {
  toggleForUser: toggleLibraryIslandForUser,
  preload: preloadLibraryIsland,
  close: closeLibraryIsland,
  openSavePopover: toggleSavePopover,
  handleCardDragStart,
  handleCardDragEnd,
  handleTriggerDragOver: handleLibraryTriggerDragOver,
  handleTriggerDragLeave: handleLibraryTriggerDragLeave,
  handleTriggerDrop: handleLibraryTriggerDrop,
} = libraryCommands
</script>
