<template>
  <div
    class="workflow-node-header"
    :class="{ 'workflow-node-header--rounded': rounded }"
    @mousedown="onHeaderMouseDown"
  >
    <div class="node-header-title">
      <span v-if="$slots.icon" class="node-header-icon">
        <slot name="icon" />
      </span>
      <span
        ref="titleTextRef"
        class="node-header-title-text"
        :class="{ 'is-editing': isEditing }"
        :contenteditable="isEditing ? 'plaintext-only' : 'false'"
        spellcheck="false"
        title="单击重命名"
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
const props = defineProps<{
  title: string
  rounded?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:title', value: string): void
}>()

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

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.node-header-icon {
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.95;
}

.node-header-icon :deep(svg) {
  display: block;
  width: 18px;
  height: 18px;
}

.node-header-title-text {
  min-width: 0;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: text;
  outline: none;
}

.node-header-title-text.is-editing {
  max-width: 220px;
  text-overflow: clip;
}

.workflow-node-header--rounded {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}
</style>
