<template>
  <footer class="floating-bottom-bar">
    <transition name="slide-up">
      <div v-if="showSettings" class="settings-panel">
        <div class="settings-section">
          <div class="section-header">
            <span class="section-title">Aspect Ratio</span>
            <div class="ratio-group">
              <button
                v-for="ratio in ratioOptions"
                :key="ratio"
                type="button"
                class="ratio-btn"
                :class="{ active: selectedRatio === ratio }"
                @click="emit('select-ratio', ratio)"
              >
                {{ ratio }}
              </button>
            </div>
          </div>
        </div>

        <div class="settings-section">
          <div class="section-header">
            <span class="section-title">Quality</span>
            <div class="ratio-group">
              <button
                v-for="quality in qualityOptions"
                :key="quality"
                type="button"
                class="ratio-btn"
                :class="{ active: selectedQuality === quality }"
                @click="emit('select-quality', quality)"
              >
                {{ quality.toUpperCase() }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </transition>

    <div v-if="selectedPreviewItems.length > 0" class="selected-preview-bar" @mousedown.stop>
      <div class="selected-preview-header">
        <span class="selected-preview-count">{{ selectedPreviewItems.length }} selected</span>
        <button class="clear-selected-btn" type="button" @click="emit('clear-selected')">Clear</button>
      </div>
      <div class="selected-preview-list">
        <button
          v-for="item in visiblePreviewItems"
          :key="item.id"
          type="button"
          class="selected-preview-item"
          @click="emit('focus-selected', item.id)"
        >
          <img :src="item.url" class="selected-preview-image" />
          <span class="selected-preview-remove" @click.stop="emit('remove-selected', item.id)">×</span>
        </button>
        <div v-if="previewOverflowCount > 0" class="selected-preview-overflow">+{{ previewOverflowCount }}</div>
      </div>
    </div>

    <div class="prompt-container" @mousedown.stop>
      <button type="button" class="add-btn" :class="{ active: showSettings }" @click="emit('toggle-settings')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      </button>
      <input
        :value="userPrompt"
        type="text"
        :placeholder="promptPlaceholder"
        @input="onPromptInput"
        @keyup.enter="emit('generate')"
      />
      <button type="button" class="send-btn" :disabled="directGenerating" @click="emit('generate')">
        <svg v-if="!directGenerating" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
        <n-spin v-else size="small" />
      </button>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NSpin } from 'naive-ui'
import type { GenerateQualityOption, PatternGenerateRatioOption } from '../nodes/nodeOptionConfig'

type SelectedPreviewItem = {
  id: number
  url: string
}

const props = defineProps<{
  showSettings: boolean
  ratioOptions: readonly PatternGenerateRatioOption[]
  qualityOptions: readonly GenerateQualityOption[]
  selectedRatio: PatternGenerateRatioOption
  selectedQuality: GenerateQualityOption
  selectedPreviewItems: SelectedPreviewItem[]
  userPrompt: string
  promptPlaceholder: string
  hasDirectPrompt: boolean
  directGenerating: boolean
}>()

const emit = defineEmits<{
  (event: 'toggle-settings'): void
  (event: 'select-ratio', value: PatternGenerateRatioOption): void
  (event: 'select-quality', value: GenerateQualityOption): void
  (event: 'clear-selected'): void
  (event: 'focus-selected', id: number): void
  (event: 'remove-selected', id: number): void
  (event: 'update:user-prompt', value: string): void
  (event: 'generate'): void
}>()

const visiblePreviewItems = computed(() => props.selectedPreviewItems.slice(0, 6))
const previewOverflowCount = computed(() => Math.max(0, props.selectedPreviewItems.length - visiblePreviewItems.value.length))

function onPromptInput(event: Event) {
  emit('update:user-prompt', (event.target as HTMLInputElement).value)
}
</script>

<style scoped>
.floating-bottom-bar {
  position: absolute;
  bottom: 32px;
  left: 50%;
  transform: translateX(-50%);
  width: 100%;
  max-width: 600px;
  z-index: 100;
}

.settings-panel {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(226, 228, 232, 0.8);
  border-radius: 24px;
  padding: 20px;
  margin-bottom: 16px;
  max-height: 480px;
  overflow-y: auto;
}

.settings-section {
  margin-bottom: 20px;
}

.settings-section:last-child {
  margin-bottom: 0;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #202124;
}

.ratio-group {
  display: flex;
  gap: 8px;
}

.ratio-btn {
  padding: 6px 12px;
  border-radius: 8px;
  border: 1px solid #e2e4e8;
  background: #fff;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  color: #5f6368;
}

.ratio-btn:hover {
  border-color: #9278e6;
  color: #9278e6;
}

.ratio-btn.active {
  background: #9278e6;
  color: #fff;
  border-color: #9278e6;
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(20px);
}

.selected-preview-bar {
  margin-bottom: 10px;
  border-radius: 16px;
  border: 1px solid rgba(226, 228, 232, 0.95);
  background: rgba(255, 255, 255, 0.94);
  backdrop-filter: blur(10px);
  padding: 10px 12px;
}

.selected-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.selected-preview-count {
  font-size: 12px;
  font-weight: 600;
  color: #5f6368;
}

.clear-selected-btn {
  border: none;
  background: transparent;
  font-size: 12px;
  font-weight: 600;
  color: #9278e6;
  cursor: pointer;
  padding: 2px 4px;
}

.selected-preview-list {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 2px;
}

.selected-preview-list::-webkit-scrollbar {
  height: 4px;
}

.selected-preview-list::-webkit-scrollbar-thumb {
  background: #d7dbe3;
  border-radius: 999px;
}

.selected-preview-item {
  position: relative;
  flex: 0 0 38px;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  border: 2px solid #9278e6;
  padding: 0;
  background: #fff;
  overflow: hidden;
  cursor: pointer;
}

.selected-preview-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.selected-preview-remove {
  position: absolute;
  top: 0;
  right: 0;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.68);
  color: #fff;
  font-size: 11px;
  line-height: 14px;
  text-align: center;
}

.selected-preview-overflow {
  flex: 0 0 38px;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  border: 1px dashed #cfd5e3;
  color: #6f7787;
  font-size: 12px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f7f8fb;
}

.prompt-container {
  background: #fff;
  height: 56px;
  border-radius: 28px;
  padding: 0 8px;
  display: flex;
  align-items: center;
  gap: 12px;
  border: 1px solid #e2e4e8;
}

.prompt-container input {
  flex: 1;
  border: none;
  outline: none;
  font-size: 16px;
}

.add-btn,
.send-btn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.send-btn {
  background: #f1f3f9;
  color: #9278e6;
}
</style>
