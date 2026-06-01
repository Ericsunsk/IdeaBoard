<template>
  <Teleport to="body">
    <Transition name="admin-editor-fade">
      <div v-if="show" class="admin-editor-overlay" @click.self="$emit('close')">
        <div class="admin-editor-modal" :style="modalStyle">
          <button class="admin-editor-close" type="button" @click="$emit('close')">
            <Xmark class="admin-close-icon" />
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
import { Xmark } from '@iconoir/vue'

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
  /* Teleported modals must carry the admin tokens for slotted page content. */
  --admin-card: rgba(255, 255, 255, 0.96);
  --admin-card-strong: #ffffff;
  --admin-border: #d9dfec;
  --admin-border-soft: #e2e4e8;
  --admin-border-hover: #c8d0e4;
  --admin-surface-muted: #f4f6fb;
  --admin-surface-pressed: #eef1f8;
  --admin-surface-disabled: #f0f2f8;
  --admin-control-bg: #f4f6fb;
  --admin-control-bg-hover: #eef1f8;
  --admin-control-border: #d9dfec;
  --admin-control-border-hover: #c8d0e4;
  --admin-control-border-pressed: #bcc7df;
  --admin-text-1: #293042;
  --admin-text-2: #6e7487;
  --admin-text-3: #8a90a3;
  --admin-title-color: #293042;
  --admin-subtitle-color: #8a90a3;
  --admin-accent: #7a6bd5;
  --admin-divider: rgba(41, 48, 66, 0.08);
  --admin-success: #16a34a;
  --admin-warning: #d97706;
  --admin-danger: #dc2626;
  --admin-font-size-micro: 10px;
  --admin-font-size-caption: 11px;
  --admin-font-size-meta: 12px;
  --admin-font-size-body: 13px;
  --admin-font-size-content: 14px;
  --admin-font-size-title-sm: 15px;
  --admin-font-size-title: 16px;
  --admin-font-size-heading: 18px;
  --admin-font-size-modal-title: 26px;
  --admin-font-size-modal-title-mobile: 22px;
  --admin-line-height-tight: 1.2;
  --admin-line-height-body: 1.5;
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(41, 48, 66, 0.3);
  backdrop-filter: blur(8px);
}

.admin-editor-modal {
  width: min(980px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
  overflow: overlay;
  scrollbar-width: thin;
  scrollbar-color: rgba(116, 126, 154, 0.38) transparent;
  position: relative;
  border-radius: 22px;
  border: 1px solid var(--admin-border);
  background: var(--admin-card);
  box-shadow: 0 24px 56px rgba(41, 48, 66, 0.18);
  padding: 28px 28px 24px;
}

.admin-editor-modal::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.admin-editor-modal::-webkit-scrollbar-track {
  background: transparent;
}

.admin-editor-modal::-webkit-scrollbar-thumb {
  border: 2px solid transparent;
  border-radius: 999px;
  background: rgba(116, 126, 154, 0.38);
  background-clip: padding-box;
}

.admin-editor-modal::-webkit-scrollbar-thumb:hover {
  background: rgba(116, 126, 154, 0.56);
  background-clip: padding-box;
}

.admin-editor-modal::-webkit-scrollbar-corner {
  background: transparent;
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
  background: var(--admin-surface-muted, #f4f6fb);
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  cursor: pointer;
  transition:
    background 0.18s ease,
    transform 0.18s ease,
    color 0.18s ease,
    border-color 0.18s ease;
}

.admin-editor-close:hover {
  background: rgba(122, 107, 213, 0.1);
  border-color: var(--admin-border-hover);
  color: var(--admin-text-1);
  transform: scale(1.03);
}
.admin-close-icon {
  width: 20px;
  height: 20px;
  stroke-width: 2;
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
  background: var(--admin-surface-muted, #f4f6fb);
  color: var(--admin-text-2);
  font-size: var(--admin-font-size-caption);
}

.admin-editor-title {
  margin: 0;
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-modal-title);
  line-height: 1.15;
}

.admin-editor-desc {
  margin: 10px 0 0;
  color: var(--admin-subtitle-color);
  font-size: var(--admin-font-size-content);
  line-height: 1.7;
}

.admin-editor-footer {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  padding-top: 18px;
  margin-top: 18px;
  border-top: 1px solid var(--admin-divider);
}

.admin-editor-note {
  color: var(--admin-text-3);
  font-size: var(--admin-font-size-meta);
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
    font-size: var(--admin-font-size-modal-title-mobile);
  }
}
</style>
