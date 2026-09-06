<template>
  <div
    class="workflow-node-header"
    :class="{ 'rounded-t-xl': rounded, 'is-active': active, 'is-copy-mode': copyMode }"
    @mousedown="onHeaderMouseDown"
  >
    <div class="node-header-title">
      <span
        v-if="$slots.icon"
        class="inline-flex size-5 flex-none items-center justify-center opacity-95 [&_svg]:block [&_svg]:size-5"
      >
        <slot name="icon" />
      </span>
      <span
        ref="titleTextRef"
        class="min-w-0 max-w-[220px] cursor-text overflow-hidden truncate outline-none"
        :class="{ 'text-clip': isEditing }"
        :contenteditable="isEditing ? 'plaintext-only' : 'false'"
        spellcheck="false"
        :title="t('canvas.controls.renameHint')"
        @click.stop="startEditing"
        @mousedown.stop
        @blur="commitTitle"
        @keydown.enter.prevent="commitTitle"
        @keydown.esc.prevent="cancelEditing"
        @input="onTitleInput"
      >
        {{ displayTitle }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  title: string
  active?: boolean
  copyMode?: boolean
  rounded?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:title', value: string): void
}>()

const { t } = useI18n()

const isEditing = ref(false)
let draftTitle = ''
const titleTextRef = ref<HTMLElement | null>(null)
const displayTitle = computed(() => props.title.trim() || 'NODE')

watch(
  () => props.title,
  (title) => {
    if (!isEditing.value) draftTitle = title
  },
  { immediate: true },
)

function onHeaderMouseDown(event: MouseEvent) {
  if (isEditing.value) return
  emit('node-mousedown', event)
}

async function startEditing() {
  if (isEditing.value) return
  draftTitle = displayTitle.value
  isEditing.value = true
  await nextTick()
  const titleElement = titleTextRef.value
  if (!titleElement) return
  titleElement.textContent = draftTitle
  titleElement.focus()
  const range = document.createRange()
  range.selectNodeContents(titleElement)
  range.collapse(false)
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(range)
}

function commitTitle() {
  if (!isEditing.value) return
  const nextTitle = (titleTextRef.value?.textContent ?? '').trim()
  isEditing.value = false
  if (nextTitle && nextTitle !== props.title) {
    emit('update:title', nextTitle)
    draftTitle = nextTitle
    return
  }
  draftTitle = props.title
  if (titleTextRef.value) titleTextRef.value.textContent = displayTitle.value
}

function cancelEditing() {
  isEditing.value = false
  draftTitle = props.title
  if (titleTextRef.value) titleTextRef.value.textContent = displayTitle.value
}

function onTitleInput() {
  draftTitle = titleTextRef.value?.textContent ?? ''
}
</script>
