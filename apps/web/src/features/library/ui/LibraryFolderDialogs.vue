<template>
  <!-- 创建新文件夹弹窗 -->
  <DialogShell
    :is-open="createOpen"
    :title="t('library.dialogs.newFolderTitle')"
    :description="t('library.dialogs.folderNameLabel')"
    :size="sharePreset.size"
    :is-busy="createSaving"
    :has-close="sharePreset.hasClose"
    :close-label="t('library.dialogs.cancel')"
    content-class="bg-popover w-[min(420px,calc(100vw-32px))]"
    @update:open="emit('update:createOpen', $event)"
  >
    <div class="min-h-0 overflow-y-auto">
      <form
        class="p-6"
        @submit.prevent="submitCreate"
        @focusin="emit('focus-scope-change', true)"
        @focusout="emit('focus-scope-change', false)"
      >
        <h3 class="m-0 text-lg font-semibold">{{ t('library.dialogs.newFolderTitle') }}</h3>
        <IbFormField
          v-slot="{ ariaInvalid, describedbyId, fieldId, invalid }"
          :label="t('library.dialogs.folderNameLabel')"
          :error="createError"
          field-id="island-folder-name"
          field-class="mt-5"
          label-class="block text-sm font-semibold"
        >
          <Input
            :id="fieldId"
            v-model="createDraft"
            type="text"
            maxlength="100"
            autofocus
            :class="['mt-2', ibFormInputClass, invalid ? ibFormInputErrorClass : '']"
            :invalid="invalid"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            :placeholder="t('library.dialogs.folderNamePlaceholder')"
          />
        </IbFormField>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            type="button"
            class="rounded-pill bg-transparent px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted cursor-pointer"
            @click="emit('update:createOpen', false)"
          >
            {{ t('library.dialogs.cancel') }}
          </Button>
          <Button
            type="submit"
            :is-disabled="createSaving || !createDraft.trim()"
            class="rounded-pill bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground disabled:opacity-50 cursor-pointer"
          >
            {{ t('library.dialogs.create') }}
          </Button>
        </div>
      </form>
    </div>
  </DialogShell>

  <!-- 重命名文件夹弹窗 -->
  <DialogShell
    :is-open="renameOpen"
    :title="t('library.dialogs.renameFolderTitle')"
    :description="t('library.dialogs.folderNameLabel')"
    :size="sharePreset.size"
    :is-busy="renameSaving"
    :has-close="sharePreset.hasClose"
    :close-label="t('library.dialogs.cancel')"
    content-class="bg-popover w-[min(420px,calc(100vw-32px))]"
    @update:open="emit('update:renameOpen', $event)"
  >
    <div class="min-h-0 overflow-y-auto">
      <form
        class="p-6"
        @submit.prevent="submitRename"
        @focusin="emit('focus-scope-change', true)"
        @focusout="emit('focus-scope-change', false)"
      >
        <h3 class="m-0 text-lg font-semibold">{{ t('library.dialogs.renameFolderTitle') }}</h3>
        <IbFormField
          v-slot="{ ariaInvalid, describedbyId, fieldId, invalid }"
          :label="t('library.dialogs.folderNameLabel')"
          :error="renameError"
          field-id="island-rename-folder-name"
          field-class="mt-5"
          label-class="block text-sm font-semibold"
        >
          <Input
            :id="fieldId"
            v-model="renameDraft"
            type="text"
            maxlength="100"
            autofocus
            :class="['mt-2', ibFormInputClass, invalid ? ibFormInputErrorClass : '']"
            :invalid="invalid"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            :placeholder="t('library.dialogs.folderNamePlaceholder')"
          />
        </IbFormField>
        <div class="mt-6 flex justify-end gap-2">
          <Button
            type="button"
            class="rounded-pill bg-transparent px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted cursor-pointer"
            @click="emit('update:renameOpen', false)"
          >
            {{ t('library.dialogs.cancel') }}
          </Button>
          <Button
            type="submit"
            :is-disabled="renameSaving || !renameDraft.trim()"
            class="rounded-pill bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground disabled:opacity-50 cursor-pointer"
          >
            {{ t('library.dialogs.save') }}
          </Button>
        </div>
      </form>
    </div>
  </DialogShell>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import DialogShell from '../../../core/components/DialogShell.vue'
import { dialogShellPresets } from '../../../core/components/dialogShell'
import IbFormField from '../../../core/components/IbFormField.vue'
import { ibFormInputClass, ibFormInputErrorClass } from '../../../theme/formControls'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'

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
const sharePreset = dialogShellPresets.share

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
