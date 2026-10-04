<template>
  <!-- 创建新文件夹弹窗 -->
  <DialogRoot :open="createOpen" @update:open="emit('update:createOpen', $event)">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" />
      <DialogContent
        :class="cn(dialogContentBaseClass, dialogContentClass)"
        :style="{ width: 'min(420px, calc(100vw - 32px))' }"
        :aria-label="t('library.dialogs.newFolderTitle')"
        @pointer-down-outside="blockDialogOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ t('library.dialogs.newFolderTitle') }}
          </DialogDescription>
          <form
            class="p-6"
            @submit.prevent="submitCreate"
            @focusin="emit('focus-scope-change', true)"
            @focusout="emit('focus-scope-change', false)"
          >
            <h3 class="m-0 text-lg font-semibold">{{ t('library.dialogs.newFolderTitle') }}</h3>
            <label class="mt-5 block text-sm font-semibold" for="island-folder-name">{{
              t('library.dialogs.folderNameLabel')
            }}</label>
            <input
              id="island-folder-name"
              v-model="createDraft"
              type="text"
              class="mt-2 w-full rounded-xl border border-border bg-card/85 px-3 py-2.5 text-sm outline-none focus:border-primary"
              maxlength="100"
              autofocus
              :placeholder="t('library.dialogs.folderNamePlaceholder')"
            />
            <p v-if="createError" class="mt-2 mb-0 text-sm text-destructive">{{ createError }}</p>
            <div class="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                class="rounded-full bg-transparent px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted cursor-pointer"
                @click="emit('update:createOpen', false)"
              >
                {{ t('library.dialogs.cancel') }}
              </Button>
              <Button
                type="submit"
                :disabled="createSaving || !createDraft.trim()"
                class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground disabled:opacity-50 cursor-pointer"
              >
                {{ t('library.dialogs.create') }}
              </Button>
            </div>
          </form>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>

  <!-- 重命名文件夹弹窗 -->
  <DialogRoot :open="renameOpen" @update:open="emit('update:renameOpen', $event)">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" />
      <DialogContent
        :class="cn(dialogContentBaseClass, dialogContentClass)"
        :style="{ width: 'min(420px, calc(100vw - 32px))' }"
        :aria-label="t('library.dialogs.renameFolderTitle')"
        @pointer-down-outside="blockDialogOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ t('library.dialogs.renameFolderTitle') }}
          </DialogDescription>
          <form
            class="p-6"
            @submit.prevent="submitRename"
            @focusin="emit('focus-scope-change', true)"
            @focusout="emit('focus-scope-change', false)"
          >
            <h3 class="m-0 text-lg font-semibold">{{ t('library.dialogs.renameFolderTitle') }}</h3>
            <label class="mt-5 block text-sm font-semibold" for="island-rename-folder-name">{{
              t('library.dialogs.folderNameLabel')
            }}</label>
            <input
              id="island-rename-folder-name"
              v-model="renameDraft"
              type="text"
              class="mt-2 w-full rounded-xl border border-border bg-card/85 px-3 py-2.5 text-sm outline-none focus:border-primary"
              maxlength="100"
              autofocus
              :placeholder="t('library.dialogs.folderNamePlaceholder')"
            />
            <p v-if="renameError" class="mt-2 mb-0 text-sm text-destructive">{{ renameError }}</p>
            <div class="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                class="rounded-full bg-transparent px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted cursor-pointer"
                @click="emit('update:renameOpen', false)"
              >
                {{ t('library.dialogs.cancel') }}
              </Button>
              <Button
                type="submit"
                :disabled="renameSaving || !renameDraft.trim()"
                class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground disabled:opacity-50 cursor-pointer"
              >
                {{ t('library.dialogs.save') }}
              </Button>
            </div>
          </form>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog/dialogClasses'
import { cn } from '@/lib/utils'
import { ibDialogMaskClass } from '../../../shared/theme/dialogMask'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'

const props = defineProps<{
  createOpen: boolean
  renameOpen: boolean
  renameInitialName?: string
  onCreate: (name: string) => Promise<void>
  onRename: (name: string) => Promise<void>
}>()

const emit = defineEmits<{
  'update:createOpen': [val: boolean]
  'update:renameOpen': [val: boolean]
  'focus-scope-change': [focused: boolean]
}>()

const { t } = useI18n()

const dialogContentClass =
  'overflow-hidden rounded-2xl border border-border bg-card/80 backdrop-blur-md'

function blockDialogOutsideDismiss(event: Event) {
  event.preventDefault()
}

const createDraft = ref('')
const createError = ref('')
const createSaving = ref(false)

const renameDraft = ref('')
const renameError = ref('')
const renameSaving = ref(false)

watch(
  () => props.createOpen,
  (open) => {
    if (open) {
      createDraft.value = ''
      createError.value = ''
      createSaving.value = false
    }
  },
)

watch(
  () => props.renameOpen,
  (open) => {
    if (open) {
      renameDraft.value = props.renameInitialName ?? ''
      renameError.value = ''
      renameSaving.value = false
    }
  },
)

async function submitCreate() {
  const name = createDraft.value.trim()
  if (!name) return
  createSaving.value = true
  createError.value = ''
  try {
    await props.onCreate(name)
    emit('update:createOpen', false)
    createDraft.value = ''
  } catch (error) {
    createError.value = userFacingErrorCopy(error, t('library.errors.createFolderFailed'))
  } finally {
    createSaving.value = false
  }
}

async function submitRename() {
  const name = renameDraft.value.trim()
  if (!name) return
  renameSaving.value = true
  renameError.value = ''
  try {
    await props.onRename(name)
    emit('update:renameOpen', false)
  } catch (error) {
    renameError.value = userFacingErrorCopy(error, t('library.errors.renameFolderFailed'))
  } finally {
    renameSaving.value = false
  }
}
</script>
