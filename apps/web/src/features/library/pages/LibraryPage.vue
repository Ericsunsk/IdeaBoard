<template>
  <div
    class="app-scrollbar relative h-full w-full overflow-y-auto bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="relative z-[1] flex flex-wrap items-start justify-between gap-x-4">
      <StudioName /><UserWorkspaceNavigation />
      <UserAccountMenu
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      />
    </header>
    <main class="relative z-[1] mx-auto mt-12 max-w-[1240px]">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 class="m-0 text-[28px] font-semibold tracking-[-0.01em]">Library</h1>
          <p class="mt-2 mb-0 text-sm text-board-page-text-muted">Your saved visual references</p>
        </div>
        <div class="flex items-center gap-2">
          <Button
            v-if="selectedFolderRecord?.isMutable"
            unstyled
            label="Rename"
            class="rounded-full border border-board-card-border bg-board-control-bg px-3 py-2 text-sm font-semibold text-board-page-text-muted hover:text-board-page-text"
            @click="openRenameFolder"
          /><Button
            v-if="selectedFolderRecord?.isMutable"
            unstyled
            label="Delete"
            class="rounded-full border border-board-danger/40 bg-board-control-bg px-3 py-2 text-sm font-semibold text-board-danger hover:bg-board-danger/10"
            @click="deleteFolder"
          /><Button
            unstyled
            type="button"
            label="New folder"
            class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse hover:bg-board-primary-hover focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
            @click="folderDialogVisible = true"
          />
        </div>
      </div>
      <div
        v-if="workspace.foldersLoading.value"
        class="mt-8 h-10 animate-pulse rounded-full bg-board-card-border/50"
      ></div>
      <div
        v-else
        class="mt-8 flex gap-2 overflow-x-auto pb-1"
        role="tablist"
        aria-label="Library folders"
      >
        <button
          v-for="folder in workspace.folderTabs.value"
          :key="folder.queryKey"
          type="button"
          :aria-selected="workspace.selectedFolder.value === folder.queryKey"
          class="shrink-0 rounded-full border px-4 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
          :class="
            workspace.selectedFolder.value === folder.queryKey
              ? 'border-board-primary bg-board-primary text-ib-text-inverse'
              : 'border-board-card-border bg-board-control-bg text-board-page-text-muted hover:text-board-page-text'
          "
          @click="workspace.selectFolder(folder.queryKey)"
        >
          {{ folder.name }} <span class="ml-1 text-xs opacity-70">{{ folder.assetCount }}</span>
        </button>
      </div>
      <div
        v-if="workspace.errorMessage.value"
        class="mt-8 flex items-center justify-between gap-4 rounded-xl border border-board-danger/40 bg-board-card-bg px-4 py-3 text-sm"
        role="alert"
      >
        <span class="font-semibold">{{ workspace.errorMessage.value }}</span>
        <Button
          unstyled
          label="Retry"
          class="shrink-0 rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse"
          @click="workspace.retry"
        />
      </div>
      <section
        v-if="workspace.loading.value && workspace.assets.value.length === 0"
        class="mt-8 columns-2 gap-4 sm:columns-3 lg:columns-4"
      >
        <div
          v-for="n in 8"
          :key="n"
          class="mb-4 h-52 break-inside-avoid animate-pulse rounded-2xl bg-board-card-border/50"
          :class="n % 3 === 0 ? 'h-72' : ''"
        ></div>
      </section>
      <section
        v-else-if="
          !workspace.loading.value &&
          workspace.assets.value.length === 0 &&
          !workspace.errorMessage.value
        "
        class="mt-8 grid min-h-[300px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg text-center"
      >
        <div>
          <p class="m-0 font-semibold">No saved references yet</p>
          <p class="mt-2 mb-0 text-sm text-board-page-text-muted">
            Save images from Search to build your library.
          </p>
        </div>
      </section>
      <section
        v-else-if="workspace.assets.value.length > 0"
        class="relative mt-8 columns-2 gap-4 sm:columns-3 lg:columns-4"
        :aria-busy="workspace.loading.value"
      >
        <div
          v-if="workspace.loading.value"
          class="pointer-events-none absolute inset-x-0 top-0 z-10 h-1 animate-pulse bg-board-primary"
        ></div>
        <article
          v-for="asset in workspace.assets.value"
          :key="asset.id"
          class="group relative mb-4 break-inside-avoid overflow-hidden rounded-2xl border border-board-card-border bg-board-card-bg"
        >
          <button
            type="button"
            class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-board-primary"
            :aria-label="`View ${asset.title || 'saved reference'}`"
            @click="openAsset(asset)"
          >
            <div
              class="relative overflow-hidden bg-board-loading-bg"
              :style="{ aspectRatio: `${asset.width || 1} / ${asset.height || 1}` }"
            >
              <div
                v-if="failedAssetImages.has(asset.id)"
                class="grid min-h-40 place-items-center px-4 text-center text-sm text-board-page-text-muted"
              >
                Image unavailable
              </div>
              <img
                v-else
                :src="asset.preview_url"
                :alt="asset.title || 'Saved reference'"
                loading="lazy"
                class="h-full w-full object-cover transition-transform duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] group-hover:scale-[1.025]"
                @error="markAssetImageError(asset.id)"
              />
            </div>
          </button>
          <div class="flex items-center justify-between gap-2 p-3">
            <div class="min-w-0">
              <div class="truncate text-sm font-semibold">
                {{ asset.title || 'Untitled reference' }}
              </div>
              <div class="mt-1 truncate text-xs text-board-page-text-muted">
                {{ asset.author || asset.provider }}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <button
                type="button"
                class="rounded-full p-2 text-board-page-text-muted hover:bg-ib-surface-hover focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
                aria-label="Move saved reference"
                @click="openMoveAsset(asset)"
              >
                <Folder class="size-4" /></button
              ><button
                type="button"
                class="rounded-full p-2 text-board-page-text-muted hover:bg-ib-surface-hover hover:text-board-danger focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
                aria-label="Remove saved reference"
                @click="requestRemove(asset)"
              >
                <Trash class="size-4" />
              </button>
            </div>
          </div>
        </article>
      </section>
      <div v-if="workspace.hasMore.value" class="flex justify-center py-8">
        <Button
          unstyled
          label="Load more"
          class="rounded-full border border-board-card-border bg-board-control-bg px-4 py-2 text-sm font-semibold"
          :disabled="workspace.loading.value"
          @click="workspace.loadAssets(false)"
        />
      </div>
    </main>
    <Dialog
      v-model:visible="folderDialogVisible"
      modal
      :show-header="false"
      :style="dialogStyle"
      :pt="dialogPt"
      ><form class="p-6" @submit.prevent="submitFolder">
        <h2 class="m-0 text-lg font-semibold">New folder</h2>
        <label class="mt-5 block text-sm font-semibold" for="library-folder-name">Folder name</label
        ><InputText
          id="library-folder-name"
          v-model="folderDraft"
          unstyled
          class="mt-2 w-full rounded-xl border border-board-card-border bg-board-control-bg px-3 py-2.5 text-sm outline-none focus:border-board-primary"
          maxlength="100"
          autofocus
        />
        <p v-if="folderError" class="mt-2 mb-0 text-sm text-board-danger">{{ folderError }}</p>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            unstyled
            type="button"
            label="Cancel"
            class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted"
            @click="folderDialogVisible = false"
          /><Button
            unstyled
            type="submit"
            label="Create"
            :disabled="folderSaving || !folderDraft.trim()"
            class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50"
          />
        </div></form
    ></Dialog>
    <Dialog
      v-model:visible="renameDialogVisible"
      modal
      :show-header="false"
      :style="dialogStyle"
      :pt="dialogPt"
      ><form class="p-6" @submit.prevent="submitRename">
        <h2 class="m-0 text-lg font-semibold">Rename folder</h2>
        <InputText
          v-model="renameDraft"
          unstyled
          class="mt-5 w-full rounded-xl border border-board-card-border bg-board-control-bg px-3 py-2.5 text-sm outline-none focus:border-board-primary"
          maxlength="100"
          autofocus
        />
        <p v-if="renameError" class="mt-2 mb-0 text-sm text-board-danger">{{ renameError }}</p>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            unstyled
            type="button"
            label="Cancel"
            class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted"
            @click="renameDialogVisible = false"
          /><Button
            unstyled
            type="submit"
            label="Save"
            :disabled="renameSaving || !renameDraft.trim()"
            class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50"
          />
        </div></form
    ></Dialog>
    <Dialog
      v-model:visible="moveDialogVisible"
      modal
      :show-header="false"
      :style="dialogStyle"
      :pt="dialogPt"
      ><form class="p-6" @submit.prevent="submitMove">
        <h2 class="m-0 text-lg font-semibold">Move reference</h2>
        <label class="mt-5 block text-sm font-semibold" for="library-move-folder">Folder</label
        ><select
          id="library-move-folder"
          v-model="moveTarget"
          class="mt-2 w-full rounded-xl border border-board-card-border bg-board-control-bg px-3 py-2.5 text-sm outline-none focus:border-board-primary"
        >
          <option v-for="folder in moveFolders" :key="folder.queryKey" :value="folder.queryKey">
            {{ folder.name }}
          </option>
        </select>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            unstyled
            type="button"
            label="Cancel"
            class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted"
            @click="moveDialogVisible = false"
          /><Button
            unstyled
            type="submit"
            label="Move"
            :disabled="!moveTarget"
            class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50"
          />
        </div></form
    ></Dialog>
    <Dialog
      :visible="!!workspace.activeAsset.value"
      modal
      :show-header="false"
      :style="{ width: 'min(960px, calc(100vw - 32px))' }"
      :pt="dialogPt"
      @update:visible="(value) => !value && workspace.setActiveAsset(null)"
      ><div v-if="workspace.activeAsset.value" class="bg-board-surface-strong">
        <div class="flex items-center justify-between border-b border-board-card-border px-5 py-4">
          <div class="truncate text-base font-semibold">
            {{ workspace.activeAsset.value.title || 'Saved reference' }}
          </div>
          <Button
            text
            rounded
            aria-label="Close preview"
            class="!size-9 !p-0"
            @click="workspace.setActiveAsset(null)"
            ><template #icon><Xmark class="size-4" /></template
          ></Button>
        </div>
        <div class="grid max-h-[76vh] min-h-60 place-items-center bg-black/5 p-4">
          <div v-if="lightboxImageFailed" class="text-sm text-board-page-text-muted">
            Image unavailable
          </div>
          <img
            v-else
            :src="workspace.activeAsset.value.original_url"
            :alt="workspace.activeAsset.value.title || 'Saved reference'"
            class="max-h-[68vh] max-w-full object-contain"
            @error="lightboxImageFailed = true"
          />
        </div>
        <div class="px-5 py-4 text-sm text-board-page-text-muted">
          {{ workspace.activeAsset.value.author || 'Unknown author' }}
        </div>
      </div></Dialog
    >
    <UserConfirmDialog
      :visible="confirmDialog.visible.value"
      :title="confirmDialog.title.value"
      :message="confirmDialog.message.value"
      :confirm-label="confirmDialog.confirmLabel.value"
      :intent="confirmDialog.intent.value"
      :loading="confirmDialog.loading.value"
      @update:visible="confirmDialog.close"
      @confirm="confirmDialog.confirm"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { Folder, Trash, Xmark } from '@iconoir/vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import { HOME_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import UserConfirmDialog from '../../../shared/components/UserConfirmDialog.vue'
import { useUserConfirmDialog } from '../../../shared/composables/useUserConfirmDialog'
import { deleteFolderConfirmation, removeAssetConfirmation } from '../libraryConfirmations'
import { useLibraryWorkspace } from '../useLibraryWorkspace'
import type { LibraryAssetView } from '../libraryResource'
const router = useRouter()
const userStore = useUserStore()
const workspace = useLibraryWorkspace()
const confirmDialog = useUserConfirmDialog()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const accountControls = useUserAccountControls({
  navigation: { navigate: (path) => router.push(path), logout: () => logoutAndRedirect(HOME_PATH) },
})
const dialogStyle = { width: 'min(420px, calc(100vw - 32px))' }
const dialogPt = {
  root: { class: '!rounded-2xl !border-board-card-border' },
  content: { class: '!p-0' },
}
const folderDialogVisible = ref(false)
const folderDraft = ref('')
const folderError = ref('')
const folderSaving = ref(false)
const renameDialogVisible = ref(false)
const renameDraft = ref('')
const renameError = ref('')
const renameSaving = ref(false)
const moveDialogVisible = ref(false)
const moveAsset = ref<LibraryAssetView | null>(null)
const moveTarget = ref('')
const failedAssetImages = ref(new Set<number>())
const lightboxImageFailed = ref(false)
const selectedFolderRecord = computed(() =>
  workspace.folderTabs.value.find((folder) => folder.queryKey === workspace.selectedFolder.value),
)
const moveFolders = computed(() =>
  workspace.folderTabs.value.filter((folder) => folder.queryKey !== 'all'),
)
function openRenameFolder() {
  renameDraft.value = selectedFolderRecord.value?.name || ''
  renameError.value = ''
  renameDialogVisible.value = true
}
function openMoveAsset(asset: LibraryAssetView) {
  moveAsset.value = asset
  moveTarget.value =
    moveFolders.value.find((folder) => folder.queryKey !== String(asset.folder_id))?.queryKey ||
    'quick-saves'
  moveDialogVisible.value = true
}
function openAsset(asset: LibraryAssetView) {
  lightboxImageFailed.value = false
  workspace.setActiveAsset(asset)
}
function markAssetImageError(assetId: number) {
  failedAssetImages.value = new Set(failedAssetImages.value).add(assetId)
}
async function submitFolder() {
  if (!folderDraft.value.trim()) return
  folderSaving.value = true
  folderError.value = ''
  try {
    await workspace.createNewFolder(folderDraft.value.trim())
    folderDraft.value = ''
    folderDialogVisible.value = false
  } catch (error) {
    folderError.value = error instanceof Error ? error.message : '创建收藏夹失败'
  } finally {
    folderSaving.value = false
  }
}
async function submitRename() {
  if (!renameDraft.value.trim()) return
  renameSaving.value = true
  renameError.value = ''
  try {
    await workspace.renameFolder(workspace.selectedFolder.value, renameDraft.value.trim())
    renameDialogVisible.value = false
  } catch (error) {
    renameError.value = error instanceof Error ? error.message : '重命名收藏夹失败'
  } finally {
    renameSaving.value = false
  }
}
function deleteFolder() {
  const folder = selectedFolderRecord.value
  if (!folder?.isMutable) return
  confirmDialog.request(deleteFolderConfirmation(folder.name), async () => {
    await workspace.deleteFolder(workspace.selectedFolder.value)
  })
}
function requestRemove(asset: LibraryAssetView) {
  confirmDialog.request(removeAssetConfirmation(), async () => {
    await workspace.requestRemove(asset)
  })
}
async function submitMove() {
  if (!moveAsset.value || !moveTarget.value) return
  if (await workspace.moveSelectedAsset(moveAsset.value, moveTarget.value))
    moveDialogVisible.value = false
}
</script>
