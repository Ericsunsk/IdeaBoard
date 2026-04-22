<template>
  <Teleport to="body">
    <Transition name="admin-editor-fade">
      <div v-if="show" class="admin-editor-overlay" @click.self="$emit('close')">
        <div class="admin-editor-modal" :style="modalStyle">
          <button class="admin-editor-close" type="button" @click="$emit('close')">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>

          <div class="admin-editor-header">
            <div class="admin-editor-header-copy">
              <div v-if="eyebrow || badge" class="admin-editor-header-tags">
                <span v-if="eyebrow" class="admin-editor-eyebrow">{{ eyebrow }}</span>
                <span v-if="badge" class="admin-editor-badge">{{ badge }}</span>
              </div>
              <h2 class="admin-editor-title">{{ title }}</h2>
              <p v-if="description" class="admin-editor-desc">{{ description }}</p>
              <slot name="header-copy-extra" />
            </div>
            <slot name="header-aside" />
          </div>

          <slot />

          <div v-if="hasFooter" class="admin-editor-footer">
            <div class="admin-editor-note">
              <slot name="footer-note" />
            </div>
            <div class="admin-editor-actions">
              <slot name="footer-actions" />
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'

const props = defineProps<{
  show: boolean
  eyebrow: string
  title: string
  description: string
  badge?: string
  width?: string | number
}>()

defineEmits<{
  (e: 'close'): void
}>()

const slots = useSlots()

const hasFooter = computed(() => Boolean(slots['footer-note'] || slots['footer-actions']))

const modalStyle = computed(() => {
  if (props.width == null || props.width === '') {
    return null
  }
  const parsedWidth = typeof props.width === 'number' ? `${props.width}px` : props.width
  return {
    width: `min(${parsedWidth}, 100%)`,
  }
})
</script>

<style scoped>
.admin-editor-overlay {
  --admin-card: #111214;
  --admin-card-strong: #141416;
  --admin-border: #232326;
  --admin-text-1: #f4f4f5;
  --admin-text-2: #a1a1aa;
  --admin-text-3: #6b6b71;
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.56);
  backdrop-filter: blur(8px);
}

.admin-editor-modal {
  width: min(980px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
  position: relative;
  border-radius: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(18, 18, 20, 0.96);
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.42);
  padding: 28px 28px 24px;
}

.admin-editor-close {
  position: absolute;
  top: 18px;
  right: 18px;
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.05);
  color: #f4f4f5;
  cursor: pointer;
  transition: background 0.18s ease, transform 0.18s ease;
}

.admin-editor-close:hover {
  background: rgba(255, 255, 255, 0.1);
  transform: scale(1.03);
}

.admin-editor-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  padding-right: 44px;
  margin-bottom: 20px;
}

.admin-editor-header-copy {
  min-width: 0;
}

.admin-editor-header-tags {
  display: inline-flex;
  gap: 8px;
  margin-bottom: 12px;
}

.admin-editor-eyebrow,
.admin-editor-badge {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  border: 1px solid var(--admin-border);
  background: #17181a;
  color: var(--admin-text-2);
  font-size: 11px;
}

.admin-editor-title {
  margin: 0;
  color: var(--admin-text-1);
  font-size: 26px;
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.admin-editor-desc {
  margin: 10px 0 0;
  color: var(--admin-text-2);
  font-size: 14px;
  line-height: 1.7;
}

.admin-editor-footer {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  padding-top: 18px;
  margin-top: 18px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.admin-editor-note {
  color: var(--admin-text-3);
  font-size: 12px;
  line-height: 1.6;
  max-width: 480px;
}

.admin-editor-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.admin-editor-fade-enter-active,
.admin-editor-fade-leave-active {
  transition: opacity 0.22s ease;
}

.admin-editor-fade-enter-from,
.admin-editor-fade-leave-to {
  opacity: 0;
}

@media (max-width: 980px) {
  .admin-editor-header,
  .admin-editor-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .admin-editor-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 720px) {
  .admin-editor-overlay {
    padding: 12px;
  }

  .admin-editor-modal {
    max-height: calc(100vh - 24px);
    padding: 20px 18px 18px;
    border-radius: 20px;
  }

  .admin-editor-header {
    padding-right: 32px;
  }

  .admin-editor-title {
    font-size: 22px;
  }
}
</style>
