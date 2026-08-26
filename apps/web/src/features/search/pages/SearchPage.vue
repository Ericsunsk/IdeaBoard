<template>
  <div
    ref="pageScrollRef"
    class="app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="sticky top-0 z-40 flex items-center justify-between pointer-events-none pb-2">
      <StudioName class="pointer-events-auto" />
      <UserWorkspaceNavigation class="pointer-events-auto" />
      <UserAccountMenu
        class="pointer-events-auto"
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      />
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
            class="min-w-0 flex-1 rounded-full border border-board-card-border bg-board-control-bg/80 p-1.5 backdrop-blur-md"
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
                    :class="isReferencePending(reference.key) ? 'opacity-60' : ''"
                    :title="`Remove reference (${referenceSlotCount}/4)`"
                    :aria-label="`Remove reference ${reference.key}`"
                    :aria-busy="isReferencePending(reference.key)"
                    @click="removeReference(reference.key)"
                  >
                    <img
                      :src="reference.previewUrl"
                      alt="Search reference"
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
                        alt="Reference preview"
                        class="size-full block object-cover select-none pointer-events-none"
                      />
                    </div>
                  </div>
                </div>

                <button
                  v-if="canAddReference"
                  type="button"
                  class="search-reference-control"
                  :title="`Add more references (${referenceSlotCount}/4)`"
                  :aria-label="`Add more references (${referenceSlotCount}/4)`"
                  @click="openFilePicker"
                >
                  <Plus class="size-3.5" aria-hidden="true" />
                </button>
              </div>

              <InputText
                v-model="draftQuery"
                unstyled
                class="min-w-0 flex-1 bg-transparent px-1 text-[13.5px] text-board-control-input-text outline-none"
                :placeholder="
                  references.length > 0
                    ? 'Add keywords or prompt (optional)...'
                    : 'Search inspiration...'
                "
                :aria-label="t('search.inputLabel')"
                @input="queryError = ''"
              />

              <div class="flex items-center gap-1.5 shrink-0">
                <button
                  v-if="references.length > 0 || draftQuery"
                  type="button"
                  class="search-reference-control"
                  title="Clear search"
                  aria-label="Clear search"
                  @click="handleClearSearch"
                >
                  <Xmark class="size-3.5" aria-hidden="true" />
                </button>
                <button
                  v-if="references.length === 0"
                  type="button"
                  class="h-8 px-3 rounded-full border border-board-card-border text-board-page-text-muted hover:text-board-page-text hover:border-board-primary hover:bg-board-primary/10 flex items-center gap-1.5 text-xs font-medium transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                  aria-label="Add search reference image"
                  title="Add search reference image"
                  :disabled="!canAddReference"
                  @click="openFilePicker"
                >
                  <Camera class="size-3.5 text-board-primary" aria-hidden="true" />
                  <span class="hidden sm:inline">Image</span>
                </button>
                <Button
                  type="submit"
                  label="Search"
                  :loading="visionState === 'extracting'"
                  :disabled="hasPendingReferences || visionState === 'extracting'"
                  :title="searchButtonTitle"
                  class="!h-8 !min-w-[68px] !rounded-full !border-0 !bg-board-primary !px-4 !text-xs !font-bold !text-ib-text-inverse !shadow-none hover:!bg-board-primary-hover cursor-pointer disabled:!cursor-wait disabled:!opacity-60"
                />
              </div>
            </div>
          </form>

          <button
            type="button"
            class="group grid size-11 shrink-0 place-items-center rounded-full border border-board-card-border bg-board-control-bg/80 backdrop-blur-md transition-[transform,border-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105 active:scale-95 hover:border-board-card-border-hover focus-visible:outline-none cursor-pointer"
            :class="[isTriggerHoveredDuringDrag ? '!scale-110 !border-board-primary' : '']"
            aria-label="Toggle My Library"
            title="My Library / Drop here to Quick Save"
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
              class="size-4 text-board-page-text-muted transition-[transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover:text-board-page-text group-hover:rotate-6 group-hover:scale-105"
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
          aria-label="Upload reference images"
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
            <span>Unable to load results</span>
            <button type="button" class="font-semibold text-board-primary" @click="reload">
              Retry
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
      <section
        v-else-if="hasError && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg/80 backdrop-blur-sm text-center"
      >
        <div>
          <p class="m-0 font-semibold">Unable to load inspiration</p>
          <Button
            label="Retry"
            class="!mt-4 !rounded-full !border-0 !bg-board-primary !text-ib-text-inverse"
            @click="reload"
          />
        </div>
      </section>
      <section
        v-else-if="results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg/80 backdrop-blur-sm"
      >
        <p class="m-0 text-sm text-board-page-text-muted">{{ t('search.noResults') }}</p>
      </section>

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
          <article
            :key="`${result.provider}:${result.id}`"
            :style="style"
            draggable="true"
            class="group relative overflow-hidden rounded-[var(--search-card-radius)] bg-board-control-bg/80 shadow-xs transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-[var(--shadow-ib-panel)] cursor-grab active:cursor-grabbing select-none"
            @dragstart="handleCardDragStart($event, result)"
            @dragend="handleCardDragEnd"
          >
            <button
              type="button"
              class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-board-primary"
              :aria-label="t('search.viewResult', { title: result.title || t('search.fallbackTitle') })"
              @click="openLightbox(result)"
            >
              <div
                class="relative overflow-hidden bg-board-loading-bg"
                :style="{ aspectRatio: `${result.width} / ${result.height}` }"
              >
                <img
                  :key="`${result.provider}:${result.id}:${imageRetryCount(result.id)}`"
                  :src="result.preview_url || result.thumbnail_url"
                  :srcset="`${result.thumbnail_url} 400w, ${result.preview_url} 1200w`"
                  sizes="(min-width: 1280px) 20vw, (min-width: 1024px) 25vw, (min-width: 640px) 33vw, 100vw"
                  :alt="result.title || t('search.fallbackTitle')"
                  :loading="index < 4 ? 'eager' : 'lazy'"
                  :fetchpriority="index < 2 ? 'high' : 'auto'"
                  class="block h-full w-full object-cover select-none pointer-events-none"
                  @error="markImageError(result.id)"
                />
              </div>
            </button>

            <!-- 🌟 Hover 渐变半透操作浮层 (顶栏左上角多选对勾 + 右上角搜图 + 底栏标题/作者 + 存入个人素材库) -->
            <div
              class="pointer-events-none absolute inset-0 z-20 flex flex-col justify-between p-3.5 transition-opacity duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] bg-gradient-to-t from-black/80 via-transparent to-black/20 group-hover:opacity-100"
              :class="isResultSelected(result) ? 'opacity-100' : 'opacity-0'"
            >
              <!-- 顶栏：左上角极简多选对勾 + 右上角搜图参考圆纽 (严格统一在同一水平基准线) -->
              <div class="pointer-events-auto flex items-center justify-between">
                <!-- 左上角极简多选微圆对勾 (强制 28x28 正圆) -->
                <button
                  type="button"
                  class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full transition-colors duration-[var(--motion-control-duration)] cursor-pointer focus-visible:outline-none"
                  :class="
                    isResultSelected(result)
                      ? '!bg-board-primary text-ib-text-inverse shadow-xs'
                      : 'bg-[var(--search-overlay-btn-bg)] text-white/80 hover:bg-[var(--search-overlay-btn-bg-hover)] hover:text-white backdrop-blur-[var(--search-glass-blur)]'
                  "
                  :title="isResultSelected(result) ? 'Deselect image' : 'Select image'"
                  :aria-label="isResultSelected(result) ? 'Deselect image' : 'Select image'"
                  @click.stop="toggleBatchSelect(result)"
                >
                  <Check class="size-3.5 stroke-[2.5]" aria-hidden="true" />
                </button>

                <!-- 右上角以图搜图参考圆纽 (强制 28x28 正圆) -->
                <button
                  type="button"
                  class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full bg-[var(--search-overlay-btn-bg)] text-white backdrop-blur-[var(--search-glass-blur)] transition-colors hover:bg-board-primary focus-visible:outline-none cursor-pointer"
                  :class="{
                    '!bg-board-primary shadow-xs':
                      references.some(
                        (ref) => ref.key === searchReferenceKey(result.provider, result.id),
                      ) || isReferencePending(searchReferenceKey(result.provider, result.id)),
                  }"
                  title="Use as search reference"
                  aria-label="Use as search reference"
                  :aria-busy="isReferencePending(searchReferenceKey(result.provider, result.id))"
                  @click.stop="handleSearchResultReference(result)"
                >
                  <Search
                    class="size-3.5"
                    :class="
                      isReferencePending(searchReferenceKey(result.provider, result.id))
                        ? 'animate-pulse'
                        : ''
                    "
                    aria-hidden="true"
                  />
                </button>
              </div>

              <!-- 底部信息与存入素材库按钮 -->
              <div class="pointer-events-auto flex items-end justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <p class="m-0 truncate text-xs font-medium text-white leading-tight">
                    {{ result.title || 'Untitled reference' }}
                  </p>
                  <p class="mt-1 mb-0 truncate text-[11px] text-white/70 leading-tight">
                    by {{ result.author }} · {{ result.provider }}
                  </p>
                </div>

                <button
                  type="button"
                  class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full shadow-md transition focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] disabled:cursor-not-allowed disabled:opacity-60 cursor-pointer"
                  :class="
                    savedKeys.has(libraryAssetKey(result.provider, result.id))
                      ? 'bg-ib-success text-ib-text-inverse hover:bg-ib-success/90'
                      : 'bg-board-primary text-ib-text-inverse hover:bg-board-primary-hover shadow-board-primary/30'
                  "
                  :aria-label="
                    savedKeys.has(libraryAssetKey(result.provider, result.id))
                      ? 'Remove from Library'
                      : 'Save to Library'
                  "
                  :title="
                    savedKeys.has(libraryAssetKey(result.provider, result.id))
                      ? 'Remove from Library'
                      : 'Save to Library Folder'
                  "
                  @click.stop="toggleSavePopover($event, result)"
                >
                  <BookmarkSolid
                    v-if="savedKeys.has(libraryAssetKey(result.provider, result.id))"
                    class="size-3.5"
                  />
                  <Plus v-else class="size-3.5 stroke-[2.5]" />
                </button>
              </div>
            </div>

            <div
              v-if="failedImages.has(result.id)"
              class="grid place-items-center gap-2 bg-board-loading-bg px-4 py-5 text-center"
            >
              <span class="text-xs font-semibold text-board-page-text-muted">{{ t('common.errors.imageLoadFailed') }}</span>
              <Button
                :label="t('common.actions.retry')"
                size="small"
                severity="secondary"
                outlined
                class="!rounded-full"
                @click="retryImage(result.id)"
              />
            </div>
          </article>
        </template>
      </SearchResultViewport>

      <div v-if="loadingMore" class="flex justify-center py-5">
        <ProgressSpinner
          class="!h-7 !w-7 text-board-primary"
          stroke-width="4"
          :aria-label="t('search.loadingMore')"
        />
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

    <!-- 🌟 Demo 同款零遮挡全景高清 Lightbox 弹窗 (bg-black/65 backdrop-blur-2xl) -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition-opacity duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
        leave-active-class="transition-opacity duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
        enter-from-class="opacity-0"
        leave-to-class="opacity-0"
      >
        <div
          v-if="lightboxVisible && activeResult"
          class="fixed inset-0 z-[2000] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
          @click="closeLightbox"
        >
          <!-- 顶部极简悬浮信息与操作栏 -->
          <div
            class="flex w-full max-w-6xl items-center justify-between z-20 shrink-0 mb-3 px-2"
            @click.stop
          >
            <!-- 左上角：纯净悬浮文字 (Title + Author) -->
            <div class="min-w-0 pr-4">
              <h3
                class="m-0 text-white text-base sm:text-lg font-semibold tracking-tight drop-shadow-md truncate max-w-[240px] sm:max-w-md"
              >
                {{ activeResult.title || 'Inspiration' }}
              </h3>
              <p class="mt-1 mb-0 text-white/70 text-xs drop-shadow-md">
                {{ activeResult.author }} · {{ activeResult.provider }}
              </p>
            </div>

            <!-- 右上角：来源链接 + 极简圆形 ✕ 关闭按钮 (通透轻量毛玻璃 + Search 同款紫色 Hover) -->
            <div class="flex items-center gap-2">
              <a
                v-if="activeResult.source_url"
                :href="activeResult.source_url"
                target="_blank"
                rel="noreferrer"
                class="h-7 px-3 rounded-full bg-white/10 text-white/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-board-primary hover:text-white hover:border-transparent flex items-center justify-center text-xs font-medium no-underline cursor-pointer"
              >
                <span>View Source</span>
              </a>

              <button
                type="button"
                class="grid size-7 place-items-center rounded-full bg-white/10 text-white/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-board-primary hover:text-white hover:border-transparent focus-visible:outline-none cursor-pointer"
                aria-label="Close preview"
                title="Close (Esc)"
                @click="closeLightbox"
              >
                <Xmark class="size-3.5" aria-hidden="true" />
              </button>
            </div>
          </div>

          <!-- 中间大图展示区 (彻底去除边框与遮挡，纯粹沉浸大图鉴赏) -->
          <div
            class="flex-1 w-full max-w-6xl flex items-center justify-center overflow-hidden min-h-0"
            @click.stop
          >
            <img
              :src="activeResult.original_url || activeResult.preview_url"
              :alt="activeResult.title || t('search.fallbackTitle')"
              class="max-h-[82vh] max-w-full object-contain rounded-2xl shadow-2xl border-0 ring-0 outline-none select-none"
            />
          </div>
        </div>
      </Transition>
    </Teleport>

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
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import { Bookmark, BookmarkSolid, Camera, Check, Plus, Search, Xmark } from '@iconoir/vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import { HOME_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import SearchSavePopover from '../components/SearchSavePopover.vue'
import SearchBatchActionDock from '../components/SearchBatchActionDock.vue'
import SearchResultViewport from '../components/SearchResultViewport.vue'
import { libraryAssetKey } from '../../library/libraryResource'
import { searchReferenceKey } from '../searchLibraryBridge'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import {
  SearchLibraryFloatingIsland as LibraryFloatingIsland,
  useSearchScreenRuntime,
} from '../useSearchScreenRuntime'

const router = useRouter()
const { t } = useI18n()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const accountControls = useUserAccountControls({
  navigation: { navigate: (path) => router.push(path), logout: () => logoutAndRedirect(HOME_PATH) },
})
const pageScrollRef = ref<HTMLElement | null>(null)
const userPublicID = computed(() => userStore.user?.public_id)
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
  hasPendingReferences,
  referenceSlotCount,
  canAddReference,
  fileInput,
  visionState,
  buttonTitle: searchButtonTitle,
} = search
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
  isReferencePending,
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
