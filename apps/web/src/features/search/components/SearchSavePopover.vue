<template>
  <div class="absolute top-3 right-3 z-[1]">
    <button
      type="button"
      class="grid size-9 place-items-center rounded-full bg-board-overlay-bg text-board-page-text transition-colors hover:bg-ib-surface-hover focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] disabled:opacity-60"
      :aria-label="saved ? 'Saved to Library' : 'Save to Library'"
      :disabled="saving"
      @click.stop="toggle"
    >
      <Check v-if="saved" class="size-4 text-board-primary" />
      <Plus v-else class="size-4" />
    </button>
    <Popover ref="popover" :pt="{ root: { class: '!rounded-xl !border-board-card-border !bg-board-overlay-bg !shadow-[var(--shadow-ib-panel)]' }, content: { class: '!p-2' } }">
      <div class="w-52">
        <button
          v-for="folder in folders"
          :key="folder.id"
          type="button"
          class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm font-semibold text-board-page-text hover:bg-ib-surface-hover disabled:opacity-50"
          :disabled="saving"
          @click="saveTo(folder.id)"
        >
          <span class="truncate">{{ folder.name }}</span><span class="ml-2 text-xs text-board-page-text-muted">{{ folder.asset_count }}</span>
        </button>
        <button type="button" class="mt-1 w-full rounded-lg px-3 py-2 text-left text-sm font-semibold text-board-primary hover:bg-ib-surface-hover" @click="creating = true">New folder</button>
        <form v-if="creating" class="mt-2 border-t border-board-card-border pt-2" @submit.prevent="saveToNewFolder">
          <InputText v-model="folderName" unstyled autofocus maxlength="100" placeholder="Folder name" class="w-full rounded-lg border border-board-card-border bg-board-control-bg px-3 py-2 text-sm outline-none focus:border-board-primary" />
          <button type="submit" class="mt-2 w-full rounded-lg bg-board-primary px-3 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50" :disabled="saving || !folderName.trim()">Create & save</button>
        </form>
      </div>
    </Popover>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import InputText from 'primevue/inputtext'
import Popover from 'primevue/popover'
import { Check, Plus } from '@iconoir/vue'
import type { SearchResult } from '../searchResource'
import { getLibraryFolders, saveSearchReference, type LibraryFolderView } from '../../library/libraryResource'
import { useAppToast } from '../../../shared/composables/useAppToast'

const props = defineProps<{ result: SearchResult; saved?: boolean }>()
const emit = defineEmits<{ saved: [] }>()
const toast = useAppToast(); const popover = ref<InstanceType<typeof Popover>>(); const folders = ref<LibraryFolderView[]>([]); const saving = ref(false); const creating = ref(false); const folderName = ref('')
async function ensureFolders() { if (folders.value.length) return; try { folders.value = await getLibraryFolders() } catch (error) { toast.error(error instanceof Error ? error.message : '加载收藏夹失败') } }
async function toggle(event: Event) { if (props.saved) return; await ensureFolders(); popover.value?.toggle(event) }
async function persist(folderId?: number, newFolderName?: string) { saving.value = true; try { await saveSearchReference(props.result, folderId, newFolderName); emit('saved'); popover.value?.hide(); toast.success('Saved to Library') } catch (error) { toast.error(error instanceof Error ? error.message : '保存素材失败') } finally { saving.value = false } }
function saveTo(folderId: number) { void persist(folderId) }
function saveToNewFolder() { const name = folderName.value.trim(); if (name) void persist(undefined, name) }
</script>
